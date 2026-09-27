package httpapi

// Tests of the meeting endpoints over a fake module: the request is passed on as the
// module expects it, the errors become the contract codes, the JSON has the
// contract shape.

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"strings"
	"testing"
	"time"

	"maxhackathon/backend/internal/initiatives"
	"maxhackathon/backend/internal/meeting"
	"maxhackathon/backend/internal/registry"
)

type fakeMeetings struct {
	view     meeting.View
	tracker  meeting.Tracker
	received meeting.ReceivedBallot
	err      error

	created meeting.CreateInput
	viewer  string
}

func (f *fakeMeetings) Create(_ context.Context, in meeting.CreateInput) (meeting.View, error) {
	f.created = in

	return f.view, f.err
}

func (f *fakeMeetings) Get(_ context.Context, _, viewerID string) (meeting.View, error) {
	f.viewer = viewerID

	return f.view, f.err
}

func (f *fakeMeetings) Tracker(context.Context, string, string) (meeting.Tracker, error) {
	return f.tracker, f.err
}

func (f *fakeMeetings) ReceiveBallot(context.Context, string, string, string) (meeting.ReceivedBallot, error) {
	return f.received, f.err
}

func newMeetingServer(m *fakeMeetings) http.Handler {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	return NewHandler(Deps{
		Auth: &Authenticator{BotToken: "t", MaxAge: time.Hour, DevMode: true,
			Users: &fakeUsers{seen: map[int64]bool{}}, Log: log, Now: time.Now},
		Houses: fakeHouses{}, Profiles: fakeProfiles{}, DB: nilReadiness{}, Log: log,
		Meetings: m,
	})
}

func sampleMeetingView() meeting.View {
	notice := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)

	return meeting.View{
		Meeting: meeting.Meeting{
			ID: "m-1", InitiativeID: "i-1", Attempt: 1, Form: meeting.FormPaperAbsentee, Status: meeting.StatusNotice,
			ChairOwnerID: "o-1", SecretaryOwnerID: "o-2",
			NoticeAt: notice, VotingStartsAt: notice.Add(10 * 24 * time.Hour), VotingEndsAt: notice.Add(20 * 24 * time.Hour),
		},
		Title:     "Камеры в подъездах",
		House:     registry.HouseRef{ID: "h-1", Address: "г. Казань, ул. Демонстрационная, д. 1"},
		Chair:     meeting.Officer{OwnerID: "o-1", MaskedName: "Иванов И. И."},
		Secretary: meeting.Officer{OwnerID: "o-2", MaskedName: "Петрова А. В."},
		Agenda: []initiatives.AgendaItem{
			{ID: "a-1", Position: 1, Text: "Установить видеонаблюдение", MajorityRule: "two_thirds_of_all"},
		},
		Progress: meeting.Progress{
			BallotsTotal: 64, BallotsReceived: 1,
			ParticipantsM2: big.NewRat(2615, 100), TotalM2: big.NewRat(3000, 1), QuorumAboveM2: big.NewRat(1500, 1),
		},
		IsAdmin: true,
	}
}

const createMeetingBody = `{"form": "paper_absentee", "notice_at": "2026-10-05T12:00:00+03:00",
	"voting_starts_at": "2026-10-15T09:00:00+03:00", "voting_ends_at": "2026-10-25T20:00:00+03:00",
	"chair_owner_id": "o-1", "secretary_owner_id": "o-2"}`

func TestCreateMeeting(t *testing.T) {
	fake := &fakeMeetings{view: sampleMeetingView()}
	rec := callJSON(newMeetingServer(fake), http.MethodPost, "/api/v1/initiatives/i-1/meetings", createMeetingBody)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	in := fake.created
	if in.InitiativeID != "i-1" || in.ByUserID != "user-42" || in.Form != meeting.FormPaperAbsentee ||
		in.ChairOwnerID != "o-1" || in.SecretaryOwnerID != "o-2" ||
		!in.NoticeAt.Equal(time.Date(2026, 10, 5, 9, 0, 0, 0, time.UTC)) {
		t.Fatalf("module got %+v", in)
	}

	var got struct {
		ID     string `json:"id"`
		Status string `json:"status"`
		Chair  struct {
			MaskedName string `json:"masked_name"`
		} `json:"chair"`
		AgendaItems []struct {
			ID           string `json:"id"`
			MajorityRule string `json:"majority_rule"`
		} `json:"agenda_items"`
		Progress map[string]any `json:"progress"`
		IsAdmin  bool           `json:"is_admin"`
		Outcome  *string        `json:"outcome"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.ID != "m-1" || got.Status != "notice" || got.Chair.MaskedName != "Иванов И. И." || !got.IsAdmin ||
		len(got.AgendaItems) != 1 || got.AgendaItems[0].ID != "a-1" || got.Outcome != nil {
		t.Fatalf("meeting JSON = %s", rec.Body)
	}
	// Areas travel as exact decimal strings.
	if got.Progress["participants_m2"] != "26.15" || got.Progress["total_m2"] != "3000.00" ||
		got.Progress["quorum_above_m2"] != "1500.00" || got.Progress["ballots_total"] != float64(64) {
		t.Fatalf("progress JSON = %v", got.Progress)
	}

	for _, body := range []string{
		`{}`,
		strings.Replace(createMeetingBody, `"paper_absentee"`, `"in_person"`, 1),
		strings.Replace(createMeetingBody, `"2026-10-05T12:00:00+03:00"`, `"5 октября"`, 1),
	} {
		rec := callJSON(newMeetingServer(&fakeMeetings{}), http.MethodPost, "/api/v1/initiatives/i-1/meetings", body)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "invalid_request") {
			t.Fatalf("body %s: status = %d, %s", body, rec.Code, rec.Body)
		}
	}
}

func TestMeetingErrors(t *testing.T) {
	tests := []struct {
		err    error
		status int
		code   string
	}{
		{meeting.ErrInitiativeNotFound, http.StatusNotFound, "initiative_not_found"},
		{meeting.ErrNotFound, http.StatusNotFound, "meeting_not_found"},
		{meeting.ErrBallotNotFound, http.StatusNotFound, "ballot_not_found"},
		{meeting.ErrStaffOnly, http.StatusForbidden, "staff_only"},
		{meeting.ErrNotMember, http.StatusForbidden, "not_member"},
		{meeting.ErrWrongStage, http.StatusConflict, "wrong_stage"},
		{meeting.ErrActiveMeetingExists, http.StatusConflict, "active_meeting_exists"},
		{meeting.ErrInvalidOfficers, http.StatusBadRequest, "invalid_officers"},
		{meeting.ErrAlreadyReceived, http.StatusConflict, "already_received"},
		{meeting.ErrVotingFinished, http.StatusConflict, "voting_finished"},
		{errors.New("connection reset"), http.StatusInternalServerError, "internal"},
	}
	for _, tt := range tests {
		rec := callJSON(newMeetingServer(&fakeMeetings{err: tt.err}), http.MethodPost,
			"/api/v1/initiatives/i-1/meetings", createMeetingBody)
		if rec.Code != tt.status || !strings.Contains(rec.Body.String(), `"`+tt.code+`"`) {
			t.Errorf("%v: status = %d, body = %s; want %d %s", tt.err, rec.Code, rec.Body, tt.status, tt.code)
		}
	}

	// A date rule names its field, so the form can highlight it.
	rec := callJSON(newMeetingServer(&fakeMeetings{err: &meeting.DatesError{Reason: meeting.DatesNoticePeriod}}),
		http.MethodPost, "/api/v1/initiatives/i-1/meetings", createMeetingBody)
	var body errorBody
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if rec.Code != http.StatusBadRequest || body.Error.Code != "invalid_dates" || body.Error.Field != "voting_starts_at" ||
		!strings.Contains(body.Error.Message, "10 дней") {
		t.Fatalf("dates error: %d %s", rec.Code, rec.Body)
	}
}

func TestMeetingReadAndBallots(t *testing.T) {
	entrance := 1
	fake := &fakeMeetings{
		view: sampleMeetingView(),
		tracker: meeting.Tracker{
			BallotsTotal: 64, Received: 1, Counted: 0, ParticipantsM2: big.NewRat(24, 1),
			Ballots: []meeting.TrackerRow{{
				BallotID: "b-1", PremiseNumber: "13", Entrance: &entrance, OwnerMaskedName: "Смирнов А. Б.",
				WeightM2: big.NewRat(24, 1), Status: meeting.BallotPaperReceived,
			}},
		},
		received: meeting.ReceivedBallot{BallotID: "b-1", Status: meeting.BallotPaperReceived,
			ReceivedAt: time.Date(2026, 10, 16, 10, 0, 0, 0, time.UTC)},
	}
	h := newMeetingServer(fake)

	rec := callJSON(h, http.MethodGet, "/api/v1/meetings/m-1", "")
	if rec.Code != http.StatusOK || fake.viewer != "user-42" || !strings.Contains(rec.Body.String(), `"voting_ends_at"`) {
		t.Fatalf("get: %d %s (viewer %q)", rec.Code, rec.Body, fake.viewer)
	}

	rec = callJSON(h, http.MethodGet, "/api/v1/meetings/m-1/tracker", "")
	var tracker struct {
		Summary map[string]any `json:"summary"`
		Ballots []struct {
			ID       string `json:"id"`
			Entrance int    `json:"entrance"`
			WeightM2 string `json:"weight_m2"`
			Status   string `json:"status"`
		} `json:"ballots"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &tracker)
	if rec.Code != http.StatusOK || tracker.Summary["received"] != float64(1) || tracker.Summary["participants_m2"] != "24.00" ||
		len(tracker.Ballots) != 1 || tracker.Ballots[0].WeightM2 != "24.00" || tracker.Ballots[0].Entrance != 1 {
		t.Fatalf("tracker: %d %s", rec.Code, rec.Body)
	}
	// The tracker never carries the choices.
	if strings.Contains(rec.Body.String(), "choice") {
		t.Fatalf("tracker shows choices: %s", rec.Body)
	}

	rec = callJSON(h, http.MethodPost, "/api/v1/meetings/m-1/ballots/receive", `{"ballot_id": "b-1"}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"status":"paper_received"`) {
		t.Fatalf("receive: %d %s", rec.Code, rec.Body)
	}
	rec = callJSON(h, http.MethodPost, "/api/v1/meetings/m-1/ballots/receive", `{}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("receive without a ballot: %d %s", rec.Code, rec.Body)
	}

	// Until Г4–Г6 land, their routes stay 501 even with the module wired.
	for _, route := range []struct{ method, target string }{
		{http.MethodPut, "/api/v1/ballots/b-1/decisions"},
		{http.MethodGet, "/api/v1/meetings/m-1/result-preview"},
		{http.MethodPost, "/api/v1/meetings/m-1/finalize"},
		{http.MethodPost, "/api/v1/meetings/m-1/demo/finish-voting"},
		{http.MethodPost, "/api/v1/meetings/m-1/demo/fill-ballots"},
	} {
		if rec := callJSON(h, route.method, route.target, `{}`); rec.Code != http.StatusNotImplemented {
			t.Fatalf("%s %s: %d", route.method, route.target, rec.Code)
		}
	}
}
