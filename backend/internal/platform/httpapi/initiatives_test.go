package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"maxhackathon/backend/internal/access"
	"maxhackathon/backend/internal/initiatives"
	"maxhackathon/backend/internal/poll"
	"maxhackathon/backend/internal/rules"
)

type fakeAccessChecks struct {
	owner  bool
	member bool
}

func (f fakeAccessChecks) IsVerifiedOwnerIn(context.Context, string, string) (bool, error) {
	return f.owner, nil
}

func (f fakeAccessChecks) MayViewInitiatives(context.Context, string, string) (bool, error) {
	return f.member, nil
}

type fakeInitiativesAPI struct {
	createErr error
	startErr  error
	started   initiatives.Initiative
	created   initiatives.CreateInput
}

func (f *fakeInitiativesAPI) CreateFromTemplate(_ context.Context, in initiatives.CreateInput) (initiatives.Initiative, error) {
	f.created = in
	if f.createErr != nil {
		return initiatives.Initiative{}, f.createErr
	}

	return initiatives.Initiative{
		ID: "init-1", HouseID: in.HouseID, Title: in.Title, Stage: "draft", RegistryVersion: 1,
		AgendaItems: []initiatives.AgendaItem{{Position: 1, Text: "Избрать председателя", MajorityRule: "majority_of_participants"}},
	}, nil
}

func (f *fakeInitiativesAPI) StartPoll(_ context.Context, _, _ string, _ time.Time) (initiatives.Initiative, error) {
	if f.startErr != nil {
		return initiatives.Initiative{}, f.startErr
	}

	return f.started, nil
}

func (f *fakeInitiativesAPI) Get(_ context.Context, _ string) (initiatives.Initiative, error) {
	return f.started, nil
}

type fakePollProgress struct {
	progress poll.Progress
}

func (f fakePollProgress) Progress(context.Context, string) (poll.Progress, error) {
	return f.progress, nil
}

type fakeVoter struct {
	err  error
	cast []poll.CastInput
}

func (f *fakeVoter) CastVote(_ context.Context, in poll.CastInput) (poll.CastResult, error) {
	f.cast = append(f.cast, in)
	if f.err != nil {
		return poll.CastResult{}, f.err
	}

	return poll.CastResult{Choice: in.Choice, PremiseNumber: "45", WeightNum: 5230, WeightDen: 2,
		UpdatedAt: time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)}, nil
}

type fakeDemoMembership struct{ err error }

func (f fakeDemoMembership) ConfirmDemoOwner(_ context.Context, _, _, _ string, _ int) (access.OwnerLink, error) {
	if f.err != nil {
		return access.OwnerLink{}, f.err
	}

	return access.OwnerLink{MembershipID: "m-1", PremiseNumber: "45"}, nil
}

type nilReadiness struct{}

func (nilReadiness) Ping(context.Context) error { return nil }

// stage1 holds the fakes of one test server; zero values are safe defaults.
type stage1 struct {
	checks   fakeAccessChecks
	inits    *fakeInitiativesAPI
	progress fakePollProgress
	votes    *fakeVoter
	demo     fakeDemoMembership
}

func (s stage1) server() http.Handler {
	if s.inits == nil {
		s.inits = &fakeInitiativesAPI{}
	}
	if s.votes == nil {
		s.votes = &fakeVoter{}
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	return NewHandler(Deps{
		Auth: &Authenticator{BotToken: "t", MaxAge: time.Hour, DevMode: true, Users: &fakeUsers{seen: map[int64]bool{}},
			Log: log, Now: time.Now},
		Houses: fakeHouses{}, Profiles: fakeProfiles{}, DB: nilReadiness{}, Log: log,
		Access: s.checks, Initiatives: s.inits, PollStarter: s.inits, PollProgress: s.progress,
		Votes: s.votes, DemoMembers: s.demo,
	})
}

func callJSON(h http.Handler, method, target, body string) *httptest.ResponseRecorder {
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, target, reader)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(HeaderDevUserID, "42")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	return rec
}

const createPath = "/api/v1/houses/h-1/initiatives"

func TestCreateInitiative(t *testing.T) {
	inits := &fakeInitiativesAPI{}
	h := stage1{checks: fakeAccessChecks{owner: true}, inits: inits}.server()

	rec := callJSON(h, http.MethodPost, createPath,
		`{"template_code":"video_surveillance","title":"Камеры","params":{"camera_count":6}}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	var created struct {
		ID              string `json:"id"`
		HouseID         string `json:"house_id"`
		Stage           string `json:"stage"`
		RegistryVersion int    `json:"registry_version"`
		AgendaItems     []struct {
			MajorityRule string `json:"majority_rule"`
		} `json:"agenda_items"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	if created.ID != "init-1" || created.HouseID != "h-1" || created.Stage != "draft" ||
		created.RegistryVersion != 1 || len(created.AgendaItems) != 1 {
		t.Fatalf("created = %s", rec.Body)
	}
	if inits.created.HouseID != "h-1" || inits.created.Params["camera_count"] != float64(6) {
		t.Fatalf("the house from the path and params must reach the module: %+v", inits.created)
	}

	cases := []struct {
		name   string
		s      stage1
		status int
		code   string
	}{
		{"not an owner", stage1{checks: fakeAccessChecks{owner: false}}, http.StatusForbidden, "not_owner"},
		{"unknown template", stage1{checks: fakeAccessChecks{owner: true},
			inits: &fakeInitiativesAPI{createErr: rules.ErrTemplateNotFound}}, http.StatusNotFound, "template_not_found"},
		{"blank title", stage1{checks: fakeAccessChecks{owner: true},
			inits: &fakeInitiativesAPI{createErr: initiatives.ErrEmptyTitle}}, http.StatusBadRequest, "invalid_request"},
		{"daily limit", stage1{checks: fakeAccessChecks{owner: true},
			inits: &fakeInitiativesAPI{createErr: initiatives.ErrTooManyInitiatives}}, http.StatusTooManyRequests, "too_many_initiatives"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := callJSON(c.s.server(), http.MethodPost, createPath, `{"template_code":"video_surveillance","title":"Камеры"}`)
			if rec.Code != c.status || !strings.Contains(rec.Body.String(), c.code) {
				t.Fatalf("status = %d, body = %s; want %d %s", rec.Code, rec.Body, c.status, c.code)
			}
		})
	}
}

func TestStartPoll(t *testing.T) {
	// The dev caller "42" is the user "user-42" (fakeUsers).
	started := initiatives.Initiative{ID: "init-1", Stage: "poll", PollEndsAt: ptrTime(time.Now().Add(24 * time.Hour)),
		InitiatorUserID: ptrString("user-42")}
	h := stage1{checks: fakeAccessChecks{owner: true}, inits: &fakeInitiativesAPI{started: started}}.server()
	endsAt := func(d time.Duration) string {
		return fmt.Sprintf(`{"ends_at":%q}`, time.Now().Add(d).Format(time.RFC3339))
	}

	for _, body := range []string{endsAt(7 * 24 * time.Hour), `{}`, ""} {
		rec := callJSON(h, http.MethodPost, "/api/v1/initiatives/init-1/start-poll", body)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"is_initiator":true`) {
			t.Fatalf("body %q: status = %d, body = %s", body, rec.Code, rec.Body)
		}
	}

	// A present body must be valid: a poll lasts from an hour to 30 days.
	for _, body := range []string{endsAt(10 * time.Minute), endsAt(40 * 24 * time.Hour), `{"ends_at":"tomorrow"}`, `{"ends_at":`} {
		rec := callJSON(h, http.MethodPost, "/api/v1/initiatives/init-1/start-poll", body)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("body %q: status = %d, want 400; body = %s", body, rec.Code, rec.Body)
		}
	}

	// is_initiator is about the caller, not about the initiative having an initiator.
	other := &fakeInitiativesAPI{started: initiatives.Initiative{ID: "init-1", Stage: "poll", InitiatorUserID: ptrString("user-7")}}
	rec := callJSON(stage1{inits: other}.server(), http.MethodPost, "/api/v1/initiatives/init-1/start-poll", `{}`)
	if !strings.Contains(rec.Body.String(), `"is_initiator":false`) {
		t.Fatalf("another initiator: body = %s", rec.Body)
	}

	for err, status := range map[error]int{
		initiatives.ErrWrongStage:   http.StatusConflict,
		initiatives.ErrNotInitiator: http.StatusForbidden,
		initiatives.ErrNotFound:     http.StatusNotFound,
	} {
		rec := callJSON(stage1{inits: &fakeInitiativesAPI{startErr: err}}.server(), http.MethodPost,
			"/api/v1/initiatives/init-1/start-poll", `{}`)
		if rec.Code != status {
			t.Fatalf("%v: status = %d, want %d", err, rec.Code, status)
		}
	}
}

func TestPollProgress(t *testing.T) {
	progress := poll.Progress{
		InitiativeID: "init-1", Stage: "poll", TotalCenti: 300000,
		ForNum: 124000, ForDen: 1, VotesFor: 8,
	}
	h := stage1{checks: fakeAccessChecks{member: true}, progress: fakePollProgress{progress: progress}}.server()

	rec := callJSON(h, http.MethodGet, "/api/v1/initiatives/init-1/poll", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	// The fields of the contract (docs/API_DESCRIPTION.md).
	var body struct {
		ForM2         string `json:"for_m2"`
		AgainstM2     string `json:"against_m2"`
		TotalM2       string `json:"total_m2"`
		DemandM2      string `json:"demand_m2"`
		DemandReached bool   `json:"demand_reached"`
		ForPercent    string `json:"for_percent"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body.ForM2 != "1240.00" || body.AgainstM2 != "0.00" || body.TotalM2 != "3000.00" ||
		body.DemandM2 != "300.00" || !body.DemandReached || body.ForPercent != "41.3" {
		t.Fatalf("progress = %s", rec.Body)
	}
	if !strings.Contains(rec.Body.String(), `"poll_ends_at":`) {
		t.Fatalf("poll_ends_at is part of the contract: %s", rec.Body)
	}

	// A guest of the house does not see the progress.
	h = stage1{checks: fakeAccessChecks{member: false}, progress: fakePollProgress{progress: progress}}.server()
	if rec := callJSON(h, http.MethodGet, "/api/v1/initiatives/init-1/poll", ""); rec.Code != http.StatusForbidden {
		t.Fatalf("not member: status = %d", rec.Code)
	}
}

func TestMyVote(t *testing.T) {
	votes := &fakeVoter{}
	h := stage1{votes: votes}.server()

	rec := callJSON(h, http.MethodPut, "/api/v1/initiatives/init-1/my-vote",
		`{"choice":"for","official_channel":"paper","willing_to_help":true}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"weight_m2":"26.15"`) ||
		!strings.Contains(rec.Body.String(), `"updated_at":"2026-09-26T12:00:00Z"`) {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	if s := votes.cast[0].Survey; s == nil || s.OfficialChannel != poll.ChannelPaper || !s.WillingToHelp {
		t.Fatalf("survey = %+v, want paper and willing", s)
	}

	// Without the survey fields the stored answers are kept: the survey is nil.
	callJSON(h, http.MethodPut, "/api/v1/initiatives/init-1/my-vote", `{"choice":"for"}`)
	if votes.cast[1].Survey != nil {
		t.Fatalf("survey = %+v, want nil", votes.cast[1].Survey)
	}

	for _, body := range []string{`{"choice":"maybe"}`, `{}`, `{"choice":"for","official_channel":"fax"}`} {
		if rec := callJSON(h, http.MethodPut, "/api/v1/initiatives/init-1/my-vote", body); rec.Code != http.StatusBadRequest {
			t.Fatalf("body %s: status = %d, want 400", body, rec.Code)
		}
	}

	for err, want := range map[error]string{
		poll.ErrNotOwner:   "not_owner",
		poll.ErrPollClosed: "poll_closed",
		poll.ErrNoWeight:   "not_in_snapshot",
	} {
		rec := callJSON(stage1{votes: &fakeVoter{err: err}}.server(), http.MethodPut,
			"/api/v1/initiatives/init-1/my-vote", `{"choice":"against"}`)
		if !strings.Contains(rec.Body.String(), want) || rec.Code < 400 || rec.Code >= 500 {
			t.Fatalf("%v: status = %d, body = %s", err, rec.Code, rec.Body)
		}
	}
}

func TestDemoMembershipEndpoint(t *testing.T) {
	rec := callJSON(stage1{}.server(), http.MethodPost, "/api/v1/houses/demo-slug/demo-membership", `{"premise_number":"45"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}

	for err, want := range map[error]int{
		access.ErrNotDemo: http.StatusForbidden,
		// Two testers on one owner record (инвариант 9): an explained 409, not a 500.
		fmt.Errorf("%w: owner o-1", access.ErrOwnerTaken): http.StatusConflict,
	} {
		rec := callJSON(stage1{demo: fakeDemoMembership{err: err}}.server(), http.MethodPost,
			"/api/v1/houses/demo-slug/demo-membership", `{"premise_number":"45"}`)
		if rec.Code != want {
			t.Fatalf("%v: status = %d, want %d; body = %s", err, rec.Code, want, rec.Body)
		}
	}
}

// A body is read into memory: an oversized one is refused before it is read.
func TestBodyLimit(t *testing.T) {
	huge := `{"template_code":"video_surveillance","title":"` + strings.Repeat("я", maxBodyBytes) + `"}`
	rec := callJSON(stage1{checks: fakeAccessChecks{owner: true}}.server(), http.MethodPost, createPath, huge)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", rec.Code)
	}
}

func ptrTime(t time.Time) *time.Time { return &t }

func ptrString(s string) *string { return &s }
