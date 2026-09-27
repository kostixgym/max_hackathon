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
	gis      meeting.GISResults
	result   meeting.Result
	final    meeting.Final
	err      error

	created   meeting.CreateInput
	viewer    string
	decisions []meeting.Decision
	gisID     string
	gisUserID string
	gisInput  meeting.GISResults
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

func (f *fakeMeetings) RecordDecisions(_ context.Context, ballotID, _ string, d []meeting.Decision) (meeting.BallotDecisions, error) {
	f.decisions = d

	return meeting.BallotDecisions{BallotID: ballotID, Status: meeting.BallotCounted, Decisions: d}, f.err
}

func (f *fakeMeetings) RecordGISResults(
	_ context.Context,
	meetingID, userID string,
	input meeting.GISResults,
) (meeting.GISResults, error) {
	f.gisID, f.gisUserID, f.gisInput = meetingID, userID, input

	return f.gis, f.err
}

func (f *fakeMeetings) Preview(context.Context, string, string) (meeting.Result, error) {
	return f.result, f.err
}

func (f *fakeMeetings) Finalize(context.Context, string, string) (meeting.Final, error) {
	return f.final, f.err
}

func (f *fakeMeetings) FinishVoting(context.Context, string, string) (meeting.View, error) {
	return f.view, f.err
}

func (f *fakeMeetings) FillBallots(context.Context, string, string) (meeting.View, error) {
	return f.view, f.err
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
		{meeting.ErrVotingNotFinished, http.StatusConflict, "voting_not_finished"},
		{meeting.ErrAlreadyFinalized, http.StatusConflict, "already_finalized"},
		{meeting.ErrBallotNotReceived, http.StatusConflict, "ballot_not_received"},
		{meeting.ErrInvalidDecisions, http.StatusBadRequest, "invalid_request"},
		{meeting.ErrInvalidGISResults, http.StatusBadRequest, "invalid_gis_results"},
		{meeting.ErrGISResultsNotAllowed, http.StatusConflict, "gis_results_not_allowed"},
		{meeting.ErrNotDemo, http.StatusForbidden, "not_demo"},
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

}

func sampleResult() meeting.Result {
	return meeting.Result{
		ParticipantsM2: big.NewRat(2400, 1), TotalM2: big.NewRat(3000, 1), QuorumReached: true,
		Items: []meeting.ItemResult{{
			AgendaItemID: "a-1", Position: 1, Text: "Установить видеонаблюдение", MajorityRule: "two_thirds_of_all",
			ForM2: big.NewRat(2040, 1), AgainstM2: big.NewRat(300, 1), AbstainM2: big.NewRat(60, 1), Accepted: true,
		}},
	}
}

func TestMeetingCountAndResult(t *testing.T) {
	fake := &fakeMeetings{
		view: sampleMeetingView(),
		gis: meeting.GISResults{
			OnlineParticipantsM2: big.NewRat(1950, 1),
			Entries: []meeting.GISResultEntry{{
				AgendaItemID: "a-1", ForM2: big.NewRat(1800, 1), AgainstM2: big.NewRat(100, 1),
				AbstainM2: big.NewRat(50, 1),
			}},
		},
		result: sampleResult(),
		final: meeting.Final{MeetingID: "m-1", Outcome: meeting.OutcomeHeld,
			FinalizedAt: time.Date(2026, 10, 26, 10, 0, 0, 0, time.UTC), Result: sampleResult()},
	}
	h := newMeetingServer(fake)

	rec := callJSON(h, http.MethodPut, "/api/v1/ballots/b-1/decisions",
		`{"decisions": [{"agenda_item_id": "a-1", "choice": "for"}, {"agenda_item_id": "a-2", "choice": "abstain"}]}`)
	if rec.Code != http.StatusOK || len(fake.decisions) != 2 || fake.decisions[1].Choice != meeting.ChoiceAbstain ||
		!strings.Contains(rec.Body.String(), `"status":"counted"`) {
		t.Fatalf("decisions: %d %s (module got %+v)", rec.Code, rec.Body, fake.decisions)
	}
	for _, body := range []string{
		`{"decisions": []}`,
		`{"decisions": [{"agenda_item_id": "a-1", "choice": "yes"}]}`,
		`{"decisions": [{"choice": "for"}]}`,
	} {
		rec := callJSON(h, http.MethodPut, "/api/v1/ballots/b-1/decisions", body)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "invalid_request") {
			t.Fatalf("decisions %s: %d %s", body, rec.Code, rec.Body)
		}
	}

	gisBody := `{"entries":[{"agenda_item_id":"a-1","for_m2":"1800.00","against_m2":"100.00",` +
		`"abstain_m2":"50.00"}],"online_participants_m2":"1950.00"}`
	rec = callJSON(h, http.MethodPut, "/api/v1/meetings/m-1/gis-results", gisBody)
	if rec.Code != http.StatusOK || fake.gisID != "m-1" || fake.gisUserID != "user-42" ||
		fake.gisInput.OnlineParticipantsM2.Cmp(big.NewRat(1950, 1)) != 0 ||
		len(fake.gisInput.Entries) != 1 || fake.gisInput.Entries[0].AgainstM2.Cmp(big.NewRat(100, 1)) != 0 ||
		!strings.Contains(rec.Body.String(), `"for_m2":"1800.00"`) {
		t.Fatalf("GIS results: %d %s (module got %+v)", rec.Code, rec.Body, fake.gisInput)
	}

	for _, body := range []string{
		`{}`,
		`{"entries":[],"online_participants_m2":"0.00"}`,
		`{"entries":[{"agenda_item_id":"a-1","for_m2":"-1","against_m2":"1","abstain_m2":"0"}],"online_participants_m2":"0"}`,
		`{"entries":[{"agenda_item_id":"a-1","for_m2":"1.001","against_m2":"0","abstain_m2":"0"}],"online_participants_m2":"1.001"}`,
		`{"entries":[{"agenda_item_id":"a-1","for_m2":"1e3","against_m2":"0","abstain_m2":"0"}],"online_participants_m2":"1000"}`,
		`{"entries":[{"agenda_item_id":"a-1","for_m2":"1/2","against_m2":"0","abstain_m2":"0"}],"online_participants_m2":"0.50"}`,
	} {
		rec := callJSON(h, http.MethodPut, "/api/v1/meetings/m-1/gis-results", body)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "invalid_request") {
			t.Fatalf("GIS body %s: %d %s", body, rec.Code, rec.Body)
		}
	}

	rec = callJSON(h, http.MethodGet, "/api/v1/meetings/m-1/result-preview", "")
	var preview struct {
		ParticipantsM2 string `json:"participants_m2"`
		QuorumReached  bool   `json:"quorum_reached"`
		AgendaResults  []struct {
			ForM2     string `json:"for_m2"`
			AbstainM2 string `json:"abstain_m2"`
			Accepted  bool   `json:"accepted"`
		} `json:"agenda_results"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &preview)
	if rec.Code != http.StatusOK || preview.ParticipantsM2 != "2400.00" || !preview.QuorumReached ||
		len(preview.AgendaResults) != 1 || preview.AgendaResults[0].ForM2 != "2040.00" ||
		preview.AgendaResults[0].AbstainM2 != "60.00" || !preview.AgendaResults[0].Accepted {
		t.Fatalf("preview: %d %s", rec.Code, rec.Body)
	}

	rec = callJSON(h, http.MethodPost, "/api/v1/meetings/m-1/finalize", "")
	var final struct {
		MeetingID   string `json:"meeting_id"`
		Outcome     string `json:"outcome"`
		FinalizedAt string `json:"finalized_at"`
		Results     []struct {
			Accepted bool `json:"accepted"`
		} `json:"results"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &final)
	if rec.Code != http.StatusOK || final.MeetingID != "m-1" || final.Outcome != "held" ||
		final.FinalizedAt != "2026-10-26T10:00:00Z" || len(final.Results) != 1 || !final.Results[0].Accepted {
		t.Fatalf("finalize: %d %s", rec.Code, rec.Body)
	}

	for _, target := range []string{"/api/v1/meetings/m-1/demo/finish-voting", "/api/v1/meetings/m-1/demo/fill-ballots"} {
		rec := callJSON(h, http.MethodPost, target, "")
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"id":"m-1"`) {
			t.Fatalf("%s: %d %s", target, rec.Code, rec.Body)
		}
		rec = callJSON(newMeetingServer(&fakeMeetings{err: meeting.ErrNotDemo}), http.MethodPost, target, "")
		if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "not_demo") {
			t.Fatalf("%s in a real house: %d %s", target, rec.Code, rec.Body)
		}
	}
}

func TestParseM2Input(t *testing.T) {
	for value, want := range map[string]*big.Rat{
		"0":       big.NewRat(0, 1),
		"0.00":    big.NewRat(0, 1),
		"1950.5":  big.NewRat(3901, 2),
		"1950.50": big.NewRat(3901, 2),
	} {
		got, ok := parseM2Input(value)
		if !ok || got.Cmp(want) != 0 {
			t.Errorf("parseM2Input(%q) = %v, %v; want %s", value, got, ok, want.RatString())
		}
	}

	for _, value := range []string{
		"", "-1", ".5", "1.", "1.001", "1e3", "1/2", "NaN", "+1", " 1", "1 ",
		"999999999999999999999999999999999999999999999999999999",
	} {
		if got, ok := parseM2Input(value); ok {
			t.Errorf("parseM2Input(%q) = %v, true; want invalid", value, got)
		}
	}
}
