package httpapi

// Tests of the sprint skeleton (К0/К2 of docs/plan-do-30-09.md): every route of
// path A exists behind the auth and answers 501 until its module lands; the
// staff cabinet endpoints work.

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"maxhackathon/backend/internal/access"
	"maxhackathon/backend/internal/registry"
)

const orgUUID = "00000000-0000-7000-8000-000000000001"

type fakeOrgs struct {
	orgs []access.OrgSummary
}

func (f fakeOrgs) OrgsByUser(_ context.Context, _ string) ([]access.OrgSummary, error) {
	return f.orgs, nil
}

func (f fakeOrgs) OrgByUser(_ context.Context, userID, orgID string) (access.OrgSummary, error) {
	for _, o := range f.orgs {
		if o.ID == orgID {
			return o, nil
		}
	}

	return access.OrgSummary{}, access.ErrForbidden
}

type fakeOrgHouses struct{ houses []registry.HouseRef }

func (f fakeOrgHouses) HousesByOrg(context.Context, string) ([]registry.HouseRef, error) {
	return f.houses, nil
}

func newSkeletonServer(orgs fakeOrgs) http.Handler {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	return NewHandler(Deps{
		Auth: &Authenticator{BotToken: "t", MaxAge: time.Hour, DevMode: true,
			Users: &fakeUsers{seen: map[int64]bool{}}, Log: log, Now: time.Now},
		Houses: fakeHouses{}, Profiles: fakeProfiles{}, DB: nilReadiness{}, Log: log,
		Orgs: orgs, OrgHouses: fakeOrgHouses{houses: []registry.HouseRef{
			{ID: "h-1", Address: "г. Казань, ул. Демо, 1", Region: "РТ", IsDemo: true},
		}},
	})
}

// Every route of path A answers 501 not_implemented until its module lands:
// the front hides such buttons, curl sees the code.
func TestSkeletonRoutesNotImplemented(t *testing.T) {
	h := newSkeletonServer(fakeOrgs{})
	empty := []struct{ method, target string }{
		{http.MethodPost, "/api/v1/initiatives/i-1/meetings"},
		{http.MethodGet, "/api/v1/meetings/m-1"},
		{http.MethodGet, "/api/v1/meetings/m-1/tracker"},
		{http.MethodPost, "/api/v1/meetings/m-1/ballots/receive"},
		{http.MethodPut, "/api/v1/ballots/b-1/decisions"},
		{http.MethodPut, "/api/v1/meetings/m-1/gis-results"},
		{http.MethodGet, "/api/v1/meetings/m-1/result-preview"},
		{http.MethodPost, "/api/v1/meetings/m-1/finalize"},
		{http.MethodGet, "/api/v1/meetings/m-1/protocol.pdf"},
		{http.MethodPost, "/api/v1/meetings/m-1/demo/finish-voting"},
		{http.MethodPost, "/api/v1/meetings/m-1/demo/fill-ballots"},
	}
	for _, route := range empty {
		t.Run(route.method+" "+route.target, func(t *testing.T) {
			rec := callJSON(h, route.method, route.target, `{}`)
			if rec.Code != http.StatusNotImplemented {
				t.Fatalf("status = %d, want 501; body = %s", rec.Code, rec.Body)
			}
			if !strings.Contains(rec.Body.String(), "not_implemented") {
				t.Fatalf("body = %s", rec.Body)
			}
		})
	}
}

func TestOrgsEndpoints(t *testing.T) {
	orgs := fakeOrgs{orgs: []access.OrgSummary{
		{ID: orgUUID, Name: "ООО «Демо-УК»", Type: "uk", Role: "operator"},
	}}
	h := newSkeletonServer(orgs)

	// /orgs: only the user's organizations.
	rec := callJSON(h, http.MethodGet, "/api/v1/orgs", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	var list struct {
		Orgs []struct {
			ID   string `json:"id"`
			Role string `json:"role"`
		} `json:"orgs"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list.Orgs) != 1 || list.Orgs[0].ID != orgUUID || list.Orgs[0].Role != "operator" {
		t.Fatalf("orgs = %s", rec.Body)
	}

	// /orgs/{id}/houses: the staff of the organization sees its houses.
	rec = callJSON(h, http.MethodGet, "/api/v1/orgs/"+orgUUID+"/houses", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	var houses struct {
		Houses []struct {
			ID      string `json:"id"`
			Address string `json:"address"`
		} `json:"houses"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &houses)
	if len(houses.Houses) != 1 || houses.Houses[0].ID != "h-1" {
		t.Fatalf("houses = %s", rec.Body)
	}

	// A foreign organization: 403.
	rec = callJSON(h, http.MethodGet, "/api/v1/orgs/00000000-0000-7000-8000-000000000002/houses", "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("foreign org: status = %d", rec.Code)
	}

	// Self-granting demo staff access has been removed.
	rec = callJSON(h, http.MethodPost, "/api/v1/houses/demo-slug/demo-staff", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("removed demo staff route: status = %d", rec.Code)
	}
}
