package httpapi

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

type adminFixture struct {
	allowed     bool
	grants      int
	maxID       int64
	orgID, role string
	grant       bool
}

func (a *adminFixture) IsSystemAdmin(context.Context, string) (bool, error) { return a.allowed, nil }
func (*adminFixture) SearchKnownUsers(context.Context, string) ([]access.ManagedUser, error) {
	return []access.ManagedUser{{MaxUserID: 12345678901234567}}, nil
}
func (*adminFixture) AllOrganizations(context.Context) ([]registry.Org, error) {
	return []registry.Org{{ID: orgUUID, Name: "УК", Type: "uk"}}, nil
}
func (a *adminFixture) SetOrgStaff(_ context.Context, maxID int64, orgID, role string, grant bool) error {
	a.grants++
	a.maxID = maxID
	a.orgID = orgID
	a.role = role
	a.grant = grant
	return nil
}

func adminTestServer(admin AdminAccess) http.Handler {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewHandler(Deps{
		Auth:   &Authenticator{BotToken: "t", MaxAge: time.Hour, DevMode: true, Users: &fakeUsers{seen: map[int64]bool{}}, Log: log, Now: time.Now},
		Houses: fakeHouses{}, Profiles: fakeProfiles{}, DB: nilReadiness{}, Log: log, Admin: admin,
	})
}

func TestAdminEndpointsRequirePersistedSystemRole(t *testing.T) {
	admin := &adminFixture{}
	h := adminTestServer(admin)
	rec := callJSON(h, http.MethodGet, "/api/v1/admin/users?q=42", "")
	if rec.Code != http.StatusForbidden || admin.grants != 0 {
		t.Fatalf("non-admin search: status=%d body=%s", rec.Code, rec.Body)
	}

	admin.allowed = true
	rec = callJSON(h, http.MethodGet, "/api/v1/admin/users?q=42", "")
	var response struct {
		Users []struct {
			MaxUserID string `json:"max_user_id"`
		} `json:"users"`
	}
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &response) != nil || len(response.Users) != 1 || response.Users[0].MaxUserID != "12345678901234567" {
		t.Fatalf("admin search: status=%d body=%s", rec.Code, rec.Body)
	}
	rec = callJSON(h, http.MethodPut, "/api/v1/admin/org-staff", `{"max_user_id":"12345678901234567","org_id":"`+orgUUID+`","role":"admin","action":"grant"}`)
	if rec.Code != http.StatusOK || admin.grants != 1 || admin.maxID != 12345678901234567 || admin.orgID != orgUUID || admin.role != "admin" || !admin.grant {
		t.Fatalf("admin grant: status=%d body=%s request=%+v", rec.Code, rec.Body, admin)
	}
	rec = callJSON(h, http.MethodGet, "/api/v1/admin/users?q=4x", "")
	if rec.Code != http.StatusBadRequest || strings.Contains(rec.Body.String(), "123456789") {
		t.Fatalf("invalid query: status=%d body=%s", rec.Code, rec.Body)
	}
}
