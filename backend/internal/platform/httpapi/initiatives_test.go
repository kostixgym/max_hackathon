package httpapi

import (
	"context"
	"encoding/json"
	"errors"
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
	created   initiatives.Initiative
	started   initiatives.Initiative
}

func (f *fakeInitiativesAPI) CreateFromTemplate(_ context.Context, in initiatives.CreateInput) (initiatives.Initiative, error) {
	if f.createErr != nil {
		return initiatives.Initiative{}, f.createErr
	}

	return initiatives.Initiative{ID: "init-1", HouseID: in.HouseID, Title: in.Title, Stage: "draft"}, nil
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

type fakeDemoMembership struct{ err error }

func (f fakeDemoMembership) ConfirmDemoOwner(_ context.Context, _, _, _ string, _ int) (access.OwnerLink, error) {
	if f.err != nil {
		return access.OwnerLink{}, f.err
	}

	return access.OwnerLink{MembershipID: "m-1", PremiseNumber: "45"}, nil
}

type nilReadiness struct{}

func (nilReadiness) Ping(context.Context) error { return nil }

func newStage1Server(checks fakeAccessChecks, inits *fakeInitiativesAPI, progress fakePollProgress) http.Handler {
	return newStage1ServerWithDemo(checks, inits, progress, fakeDemoMembership{})
}

func newStage1ServerWithDemo(checks fakeAccessChecks, inits *fakeInitiativesAPI, progress fakePollProgress,
	demo fakeDemoMembership,
) http.Handler {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	return NewHandler(Deps{
		Auth: &Authenticator{BotToken: "t", MaxAge: time.Hour, DevMode: true, Users: &fakeUsers{seen: map[int64]bool{}},
			Log: log, Now: time.Now},
		Houses: fakeHouses{}, Profiles: fakeProfiles{}, DB: nilReadiness{}, Log: log,
		Access: checks, Initiatives: inits, PollStarter: inits, PollProgress: progress,
		DemoMembers: demo,
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

func TestCreateInitiative(t *testing.T) {
	inits := &fakeInitiativesAPI{}
	h := newStage1Server(fakeAccessChecks{owner: true}, inits, fakePollProgress{})

	rec := callJSON(h, http.MethodPost, "/api/v1/initiatives",
		`{"house_id":"h-1","template_code":"cctv","title":"Камеры"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	var created struct {
		ID    string `json:"id"`
		Stage string `json:"stage"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	if created.ID != "init-1" || created.Stage != "draft" {
		t.Fatalf("created = %s", rec.Body)
	}

	// Not an owner: 403.
	h = newStage1Server(fakeAccessChecks{owner: false}, &fakeInitiativesAPI{}, fakePollProgress{})
	rec = callJSON(h, http.MethodPost, "/api/v1/initiatives",
		`{"house_id":"h-1","template_code":"cctv","title":"Камеры"}`)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "not_owner") {
		t.Fatalf("not owner: status = %d, body = %s", rec.Code, rec.Body)
	}

	// Unknown template: 404.
	bad := &fakeInitiativesAPI{createErr: rules.ErrTemplateNotFound}
	h = newStage1Server(fakeAccessChecks{owner: true}, bad, fakePollProgress{})
	rec = callJSON(h, http.MethodPost, "/api/v1/initiatives",
		`{"house_id":"h-1","template_code":"nope","title":"Камеры"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("template: status = %d", rec.Code)
	}
}

func TestStartPoll(t *testing.T) {
	// The dev caller "42" is the user "user-42" (fakeUsers).
	started := initiatives.Initiative{ID: "init-1", Stage: "poll", PollEndsAt: ptrTime(time.Now().Add(24 * time.Hour)),
		InitiatorUserID: ptrString("user-42")}
	inits := &fakeInitiativesAPI{started: started}
	h := newStage1Server(fakeAccessChecks{owner: true}, inits, fakePollProgress{})

	for _, body := range []string{`{"days":7}`, `{}`, ""} {
		rec := callJSON(h, http.MethodPost, "/api/v1/initiatives/init-1/start-poll", body)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"is_initiator":true`) {
			t.Fatalf("body %q: status = %d, body = %s", body, rec.Code, rec.Body)
		}
	}

	// A present body must be valid: the 30-day limit cannot be bypassed.
	for _, body := range []string{`{"days":365}`, `{"days":-1}`, `{"days":"7"}`, `{"days":`} {
		rec := callJSON(h, http.MethodPost, "/api/v1/initiatives/init-1/start-poll", body)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("body %q: status = %d, want 400; body = %s", body, rec.Code, rec.Body)
		}
	}

	// is_initiator is about the caller, not about the initiative having an initiator.
	other := &fakeInitiativesAPI{started: initiatives.Initiative{ID: "init-1", Stage: "poll", InitiatorUserID: ptrString("user-7")}}
	h = newStage1Server(fakeAccessChecks{owner: true}, other, fakePollProgress{})
	rec := callJSON(h, http.MethodPost, "/api/v1/initiatives/init-1/start-poll", `{}`)
	if !strings.Contains(rec.Body.String(), `"is_initiator":false`) {
		t.Fatalf("another initiator: body = %s", rec.Body)
	}

	// Wrong stage: 409.
	inits = &fakeInitiativesAPI{startErr: initiatives.ErrWrongStage}
	h = newStage1Server(fakeAccessChecks{owner: true}, inits, fakePollProgress{})
	rec = callJSON(h, http.MethodPost, "/api/v1/initiatives/init-1/start-poll", `{}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("wrong stage: status = %d", rec.Code)
	}

	// Not the initiator: 403.
	inits = &fakeInitiativesAPI{startErr: initiatives.ErrNotInitiator}
	h = newStage1Server(fakeAccessChecks{owner: true}, inits, fakePollProgress{})
	rec = callJSON(h, http.MethodPost, "/api/v1/initiatives/init-1/start-poll", `{}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("not initiator: status = %d", rec.Code)
	}
}

func TestInitiativeProgress(t *testing.T) {
	progress := poll.Progress{
		InitiativeID: "init-1", Stage: "poll", TotalCenti: 300000,
		ForNum: 124000, ForDen: 1, VotesFor: 8,
	}
	h := newStage1Server(fakeAccessChecks{member: true}, &fakeInitiativesAPI{}, fakePollProgress{progress: progress})

	rec := callJSON(h, http.MethodGet, "/api/v1/initiatives/init-1/progress", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	var body struct {
		ForAreaM2   string `json:"for_area_m2"`
		TotalAreaM2 string `json:"total_area_m2"`
		ForPercent  string `json:"for_percent"`
		Demand      struct {
			AreaM2  string `json:"area_m2"`
			Reached bool   `json:"reached"`
		} `json:"demand"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body.ForAreaM2 != "1240.00" || body.TotalAreaM2 != "3000.00" || body.ForPercent != "41.3" {
		t.Fatalf("progress = %s", rec.Body)
	}
	if body.Demand.AreaM2 != "300.00" || !body.Demand.Reached {
		t.Fatalf("demand = %s", rec.Body)
	}

	// A guest of the house does not see the progress.
	h = newStage1Server(fakeAccessChecks{member: false}, &fakeInitiativesAPI{}, fakePollProgress{progress: progress})
	rec = callJSON(h, http.MethodGet, "/api/v1/initiatives/init-1/progress", "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("not member: status = %d", rec.Code)
	}
}

func TestDemoMembershipEndpoint(t *testing.T) {
	h := newStage1Server(fakeAccessChecks{}, &fakeInitiativesAPI{}, fakePollProgress{})

	rec := callJSON(h, http.MethodPost, "/api/v1/houses/demo-slug/demo-membership", `{"premise_number":"45"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}

	demo := fakeDemoMembership{err: access.ErrNotDemo}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	h = NewHandler(Deps{
		Auth: &Authenticator{BotToken: "t", MaxAge: time.Hour, DevMode: true, Users: &fakeUsers{seen: map[int64]bool{}},
			Log: log, Now: time.Now},
		Houses: fakeHouses{}, Profiles: fakeProfiles{}, DB: nilReadiness{}, Log: log,
		Access: fakeAccessChecks{}, Initiatives: &fakeInitiativesAPI{}, PollStarter: &fakeInitiativesAPI{},
		PollProgress: fakePollProgress{}, DemoMembers: demo,
	})
	rec = callJSON(h, http.MethodPost, "/api/v1/houses/demo-slug/demo-membership", `{"premise_number":"45"}`)
	if rec.Code != http.StatusForbidden || !errors.Is(demo.err, access.ErrNotDemo) {
		t.Fatalf("not demo: status = %d", rec.Code)
	}

	// Two testers on one owner record (инвариант 9): an explained 409, not a 500.
	taken := fakeDemoMembership{err: fmt.Errorf("%w: owner o-1", access.ErrOwnerTaken)}
	h = newStage1ServerWithDemo(fakeAccessChecks{}, &fakeInitiativesAPI{}, fakePollProgress{}, taken)
	rec = callJSON(h, http.MethodPost, "/api/v1/houses/demo-slug/demo-membership", `{"premise_number":"45"}`)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "owner_taken") {
		t.Fatalf("owner taken: status = %d, body = %s", rec.Code, rec.Body)
	}
}

func ptrTime(t time.Time) *time.Time { return &t }

func ptrString(s string) *string { return &s }
