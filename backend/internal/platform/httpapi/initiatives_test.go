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
	list      []initiatives.Initiative
	details   initiatives.Initiative
}

func (f *fakeInitiativesAPI) ListByHouse(context.Context, string, string) ([]initiatives.Initiative, error) {
	return f.list, nil
}

func (f *fakeInitiativesAPI) Details(_ context.Context, id string) (initiatives.Initiative, error) {
	if id != f.details.ID {
		return initiatives.Initiative{}, initiatives.ErrNotFound
	}

	return f.details, nil
}

type fakeTemplates struct{}

func (fakeTemplates) Templates(context.Context) ([]rules.CatalogTemplate, error) {
	return []rules.CatalogTemplate{{Code: "video_surveillance", Name: "Видеонаблюдение", Version: 1, Description: "Камеры"}}, nil
}

func (fakeTemplates) TemplateByCode(_ context.Context, code string) (rules.CatalogTemplate, error) {
	if code != "video_surveillance" {
		return rules.CatalogTemplate{}, rules.ErrTemplateNotFound
	}

	return rules.CatalogTemplate{
		Code: code, Name: "Видеонаблюдение", Version: 1,
		ParamsSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`),
		UISchema:     json.RawMessage(`{"order":[]}`),
		Items: []rules.CatalogItem{{Position: 1, Text: "Избрать председателя", MajorityRule: "majority_of_participants",
			LegalReference: "ЖК РФ, ст. 46 ч. 1"}},
	}, nil
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
	mine *poll.MyVote
}

func (f *fakeVoter) MyVote(context.Context, string, string) (poll.MyVote, bool, error) {
	if f.mine == nil {
		return poll.MyVote{}, false, nil
	}

	return *f.mine, true, nil
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
		Access: s.checks, Templates: fakeTemplates{}, Initiatives: s.inits, InitiativeReader: s.inits,
		PollStarter: s.inits, PollProgress: s.progress, Votes: s.votes, DemoMembers: s.demo,
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
	var params map[string]any
	if err := json.Unmarshal(inits.created.Params, &params); err != nil || inits.created.HouseID != "h-1" ||
		params["camera_count"] != float64(6) {
		t.Fatalf("the house from the path and params must reach the module: %+v", inits.created)
	}

	// The module checks params against the template form; the answer names the field.
	tooMany := &rules.ParamsError{Field: "camera_count", Title: "Количество камер", Reason: rules.ParamsMaximum, Limit: "1000"}
	h = stage1{checks: fakeAccessChecks{owner: true}, inits: &fakeInitiativesAPI{createErr: tooMany}}.server()
	rec = callJSON(h, http.MethodPost, createPath, `{"template_code":"video_surveillance","title":"Камеры","params":{"camera_count":5000}}`)
	var apiErr struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
			Field   string `json:"field"`
		} `json:"error"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &apiErr)
	if rec.Code != http.StatusBadRequest || apiErr.Error.Code != "invalid_params" || apiErr.Error.Field != "camera_count" ||
		apiErr.Error.Message != "Поле «Количество камер»: значение не больше 1000" {
		t.Fatalf("invalid params: status = %d, body = %s", rec.Code, rec.Body)
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

	// Решение 76: a draft of another user does not exist for a member of the house.
	draft := &fakeInitiativesAPI{started: initiatives.Initiative{ID: "init-1", Stage: "draft", InitiatorUserID: ptrString("user-7")}}
	h = stage1{checks: fakeAccessChecks{member: true}, inits: draft, progress: fakePollProgress{progress: progress}}.server()
	if rec := callJSON(h, http.MethodGet, "/api/v1/initiatives/init-1/poll", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("draft of another user: status = %d, want 404", rec.Code)
	}
}

func TestTemplatesEndpoints(t *testing.T) {
	h := stage1{}.server()

	rec := callJSON(h, http.MethodGet, "/api/v1/templates", "")
	if rec.Code != http.StatusOK ||
		rec.Body.String() != `{"templates":[{"code":"video_surveillance","name":"Видеонаблюдение","version":1,"description":"Камеры"}]}` {
		t.Fatalf("templates: status = %d, body = %s", rec.Code, rec.Body)
	}

	rec = callJSON(h, http.MethodGet, "/api/v1/templates/video_surveillance", "")
	var tpl struct {
		Code         string         `json:"code"`
		ParamsSchema map[string]any `json:"params_schema"`
		UISchema     map[string]any `json:"ui_schema"`
		AgendaItems  []struct {
			MajorityRule   string `json:"majority_rule"`
			LegalReference string `json:"legal_reference"`
		} `json:"agenda_items"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &tpl)
	if rec.Code != http.StatusOK || tpl.ParamsSchema["type"] != "object" || tpl.UISchema == nil ||
		len(tpl.AgendaItems) != 1 || tpl.AgendaItems[0].LegalReference == "" {
		t.Fatalf("template: status = %d, body = %s", rec.Code, rec.Body)
	}

	if rec := callJSON(h, http.MethodGet, "/api/v1/templates/unknown", ""); rec.Code != http.StatusNotFound ||
		!strings.Contains(rec.Body.String(), "template_not_found") {
		t.Fatalf("unknown template: status = %d, body = %s", rec.Code, rec.Body)
	}
}

func TestListInitiatives(t *testing.T) {
	created := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	inits := &fakeInitiativesAPI{list: []initiatives.Initiative{
		{ID: "init-2", Title: "Мой черновик", Stage: "draft", InitiatorUserID: ptrString("user-42"), CreatedAt: created},
		{ID: "init-1", Title: "Камеры", Stage: "poll", InitiatorUserID: ptrString("user-7"), CreatedAt: created,
			PollEndsAt: ptrTime(created.Add(7 * 24 * time.Hour))},
	}}

	rec := callJSON(stage1{checks: fakeAccessChecks{member: true}, inits: inits}.server(), http.MethodGet,
		"/api/v1/houses/h-1/initiatives", "")
	var body struct {
		Initiatives []struct {
			ID          string     `json:"id"`
			Stage       string     `json:"stage"`
			Path        *string    `json:"path"`
			PollEndsAt  *time.Time `json:"poll_ends_at"`
			CreatedAt   time.Time  `json:"created_at"`
			IsInitiator bool       `json:"is_initiator"`
		} `json:"initiatives"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if rec.Code != http.StatusOK || len(body.Initiatives) != 2 || !body.Initiatives[0].IsInitiator ||
		body.Initiatives[1].IsInitiator || body.Initiatives[1].PollEndsAt == nil || !body.Initiatives[1].CreatedAt.Equal(created) {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), `"path":null`) {
		t.Fatalf("path is in the contract even before the choice: %s", rec.Body)
	}

	// An empty house is an empty list, not null.
	rec = callJSON(stage1{checks: fakeAccessChecks{member: true}}.server(), http.MethodGet, "/api/v1/houses/h-1/initiatives", "")
	if rec.Code != http.StatusOK || rec.Body.String() != `{"initiatives":[]}` {
		t.Fatalf("empty: status = %d, body = %s", rec.Code, rec.Body)
	}

	rec = callJSON(stage1{checks: fakeAccessChecks{member: false}, inits: inits}.server(), http.MethodGet,
		"/api/v1/houses/h-1/initiatives", "")
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "not_member") {
		t.Fatalf("not member: status = %d, body = %s", rec.Code, rec.Body)
	}
}

type cardJSON struct {
	ID              string `json:"id"`
	Stage           string `json:"stage"`
	IsInitiator     bool   `json:"is_initiator"`
	RegistryVersion int    `json:"registry_version"`
	TotalAreaM2     string `json:"total_area_m2"`
	Thresholds      struct {
		DemandM2    string `json:"demand_m2"`
		TwoThirdsM2 string `json:"two_thirds_m2"`
	} `json:"thresholds"`
	Template *struct {
		Code    string `json:"code"`
		Version int    `json:"version"`
	} `json:"template"`
	Params      map[string]any `json:"params"`
	AgendaItems []struct {
		MajorityRule   string `json:"majority_rule"`
		LegalReference string `json:"legal_reference"`
	} `json:"agenda_items"`
	MyVote *struct {
		Choice          string  `json:"choice"`
		WeightM2        string  `json:"weight_m2"`
		OfficialChannel *string `json:"official_channel"`
		WillingToHelp   bool    `json:"willing_to_help"`
	} `json:"my_vote"`
	AllowedActions []struct {
		Code       string `json:"code"`
		Allowed    bool   `json:"allowed"`
		ReasonCode string `json:"reason_code"`
	} `json:"allowed_actions"`
}

func (c cardJSON) action(code string) (allowed bool, reason string) {
	for _, a := range c.AllowedActions {
		if a.Code == code {
			return a.Allowed, a.ReasonCode
		}
	}

	return false, "missing"
}

func TestInitiativeCard(t *testing.T) {
	poll1 := initiatives.Initiative{
		ID: "init-1", HouseID: "h-1", Title: "Камеры", Stage: "poll", InitiatorUserID: ptrString("user-7"),
		PollEndsAt: ptrTime(time.Now().Add(time.Hour)), RegistryVersion: 1, TotalAreaCenti: 300000,
		Template: &initiatives.TemplateRef{Code: "video_surveillance", Name: "Видеонаблюдение", Version: 1},
		Params:   json.RawMessage(`{"camera_count": 6}`),
		AgendaItems: []initiatives.AgendaItem{
			{Position: 1, Text: "Избрать председателя", MajorityRule: "majority_of_participants", LegalReference: "ЖК РФ, ст. 46 ч. 1"},
			{Position: 2, Text: "Установить камеры", MajorityRule: "two_thirds_of_all", LegalReference: "ЖК РФ, ч. 1 ст. 46"},
		},
	}
	card := func(s stage1) (*httptest.ResponseRecorder, cardJSON) {
		rec := callJSON(s.server(), http.MethodGet, "/api/v1/initiatives/init-1", "")
		var c cardJSON
		_ = json.Unmarshal(rec.Body.Bytes(), &c)

		return rec, c
	}

	// A neighbour who owns a flat and voted «за» with the survey.
	rec, c := card(stage1{checks: fakeAccessChecks{owner: true, member: true}, inits: &fakeInitiativesAPI{details: poll1},
		votes: &fakeVoter{mine: &poll.MyVote{Choice: "for", WeightNum: 5230, WeightDen: 2, OfficialChannel: "gosuslugi"}}})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	if c.IsInitiator || c.RegistryVersion != 1 || c.TotalAreaM2 != "3000.00" || c.Thresholds.DemandM2 != "300.00" ||
		c.Thresholds.TwoThirdsM2 != "2000.00" || c.Template == nil || c.Template.Code != "video_surveillance" ||
		c.Params["camera_count"] != float64(6) || len(c.AgendaItems) != 2 || c.AgendaItems[1].LegalReference == "" {
		t.Fatalf("card = %s", rec.Body)
	}
	if c.MyVote == nil || c.MyVote.Choice != "for" || c.MyVote.WeightM2 != "26.15" || c.MyVote.OfficialChannel == nil ||
		*c.MyVote.OfficialChannel != "gosuslugi" {
		t.Fatalf("my_vote = %s", rec.Body)
	}
	if len(c.AllowedActions) != 7 {
		t.Fatalf("allowed_actions lists every action: %s", rec.Body)
	}
	if ok, _ := c.action("cast_poll_vote"); !ok {
		t.Fatalf("an owner votes in the poll: %s", rec.Body)
	}
	if ok, reason := c.action("start_poll"); ok || reason != "not_initiator" {
		t.Fatalf("start_poll = %v %q", ok, reason)
	}
	if ok, reason := c.action("select_path_a"); ok || reason != "not_implemented" {
		t.Fatalf("select_path_a = %v %q", ok, reason)
	}
	if strings.Contains(rec.Body.String(), `"reason_code":""`) {
		t.Fatalf("an allowed action has no reason_code: %s", rec.Body)
	}

	// A resident who is not an owner sees the card, but must confirm ownership to vote.
	_, c = card(stage1{checks: fakeAccessChecks{owner: false, member: true}, inits: &fakeInitiativesAPI{details: poll1}})
	if ok, reason := c.action("cast_poll_vote"); ok || reason != "owner_verification_required" || c.MyVote != nil {
		t.Fatalf("resident: cast_poll_vote = %v %q, my_vote = %+v", ok, reason, c.MyVote)
	}
	if rec, _ := card(stage1{checks: fakeAccessChecks{owner: false, member: true}, inits: &fakeInitiativesAPI{details: poll1}}); !strings.Contains(rec.Body.String(), `"my_vote":null`) {
		t.Fatalf("no vote is null: %s", rec.Body)
	}

	// Решение 77: after the term the owner no longer votes, the reason says why.
	ended := poll1
	ended.PollEndsAt = ptrTime(time.Now().Add(-time.Minute))
	_, c = card(stage1{checks: fakeAccessChecks{owner: true, member: true}, inits: &fakeInitiativesAPI{details: ended}})
	if ok, reason := c.action("cast_poll_vote"); ok || reason != "poll_finished" {
		t.Fatalf("after the term: cast_poll_vote = %v %q", ok, reason)
	}

	// Not a member of the house: 403.
	if rec, _ := card(stage1{checks: fakeAccessChecks{}, inits: &fakeInitiativesAPI{details: poll1}}); rec.Code != http.StatusForbidden {
		t.Fatalf("not member: status = %d", rec.Code)
	}

	// Решение 76: a draft is seen only by its initiator, who may start the poll.
	draft := poll1
	draft.Stage, draft.PollEndsAt = "draft", nil
	if rec, _ := card(stage1{checks: fakeAccessChecks{owner: true, member: true}, inits: &fakeInitiativesAPI{details: draft}}); rec.Code != http.StatusNotFound {
		t.Fatalf("draft of another user: status = %d, want 404", rec.Code)
	}
	draft.InitiatorUserID = ptrString("user-42")
	rec, c = card(stage1{checks: fakeAccessChecks{owner: true}, inits: &fakeInitiativesAPI{details: draft}})
	if rec.Code != http.StatusOK || !c.IsInitiator {
		t.Fatalf("own draft: status = %d, body = %s", rec.Code, rec.Body)
	}
	if ok, _ := c.action("start_poll"); !ok {
		t.Fatalf("the initiator starts the poll of the draft: %s", rec.Body)
	}
	if ok, reason := c.action("cast_poll_vote"); ok || reason != "wrong_stage" {
		t.Fatalf("no voting in a draft: %v %q", ok, reason)
	}

	if rec := callJSON(stage1{}.server(), http.MethodGet, "/api/v1/initiatives/unknown", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown initiative: status = %d", rec.Code)
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
