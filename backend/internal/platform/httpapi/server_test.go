package httpapi

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"testing"
	"time"

	"maxhackathon/backend/internal/access"
	"maxhackathon/backend/internal/registry"
)

const testToken = "test-bot-token"

var testNow = time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)

// signInitData signs launch parameters the way MAX does (docs: dev.max.ru/docs/webapps/validation):
// sorted "key=value" lines, secret = HMAC-SHA256(key "WebAppData", token), hash = HMAC(secret, lines).
func signInitData(token string, params map[string]string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	lines := make([]string, 0, len(keys))
	for _, k := range keys {
		lines = append(lines, k+"="+params[k])
	}

	secret := hmac.New(sha256.New, []byte("WebAppData"))
	secret.Write([]byte(token))
	m := hmac.New(sha256.New, secret.Sum(nil))
	m.Write([]byte(strings.Join(lines, "\n")))

	v := url.Values{}
	for k, val := range params {
		v.Set(k, val)
	}
	v.Set("hash", hex.EncodeToString(m.Sum(nil)))

	return v.Encode()
}

func launch(maxID int64, authAt time.Time, startParam string) map[string]string {
	p := map[string]string{
		"user":      fmt.Sprintf(`{"id":%d,"first_name":"Анна","last_name":"","language_code":"ru"}`, maxID),
		"auth_date": fmt.Sprint(authAt.Unix()),
		"query_id":  "q-1",
	}
	if startParam != "" {
		p["start_param"] = startParam
	}

	return p
}

type fakeUsers struct{ seen map[int64]bool }

func (f *fakeUsers) UpsertUser(_ context.Context, maxID int64) (access.User, error) {
	f.seen[maxID] = true

	return access.User{ID: fmt.Sprintf("user-%d", maxID), MaxUserID: maxID}, nil
}

type fakeHouses struct{}

func (fakeHouses) HouseBySlug(_ context.Context, slug string) (registry.HouseSummary, error) {
	if slug != "demo-slug" {
		return registry.HouseSummary{}, registry.ErrNotFound
	}
	v, total := 1, int64(300000)

	return registry.HouseSummary{
		ID: "house-demo", InviteSlug: slug, Address: "демо", IsDemo: true, PremisesCount: 61,
		RegistryVersion: &v, TotalAreaCenti: &total,
	}, nil
}

type fakeReadiness struct{ err error }

func (f fakeReadiness) Ping(context.Context) error { return f.err }

func newTestServer(devMode bool) (http.Handler, *fakeUsers) {
	return newTestServerWithReadiness(devMode, nil)
}

func newTestServerWithReadiness(devMode bool, readinessErr error) (http.Handler, *fakeUsers) {
	users := &fakeUsers{seen: map[int64]bool{}}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	auth := &Authenticator{
		BotToken: testToken, MaxAge: 24 * time.Hour, DevMode: devMode,
		Users: users, Log: log, Now: func() time.Time { return testNow },
	}

	return NewHandler(Deps{
		Auth: auth, Houses: fakeHouses{}, DB: fakeReadiness{err: readinessErr}, Log: log, DevMode: devMode,
	}), users
}

func do(h http.Handler, path string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	return rec
}

func TestMeWithValidInitData(t *testing.T) {
	h, users := newTestServer(false)
	initData := signInitData(testToken, launch(42, testNow.Add(-time.Minute), "demo-slug"))

	rec := do(h, "/api/v1/me", map[string]string{HeaderInitData: initData})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	if !users.seen[42] {
		t.Fatal("user must be upserted by MAX id")
	}

	var resp struct {
		User  meUser     `json:"user"`
		House *houseJSON `json:"house"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.User.ID != "user-42" || resp.User.FirstName != "Анна" {
		t.Fatalf("user = %+v", resp.User)
	}
	if resp.House == nil || resp.House.ID != "house-demo" || resp.House.Thresholds == nil {
		t.Fatalf("house by start_param expected, got %s", rec.Body)
	}
	if *resp.House.TotalAreaM2 != "3000.00" || resp.House.Thresholds.DemandM2 != "300.00" ||
		resp.House.Thresholds.QuorumAboveM2 != "1500.00" || resp.House.Thresholds.TwoThirdsM2 != "2000.00" {
		t.Fatalf("thresholds = %s", rec.Body)
	}
}

func TestInitDataRejected(t *testing.T) {
	h, _ := newTestServer(false)

	cases := map[string]string{
		"wrong token":      signInitData("another-token", launch(42, testNow, "")),
		"expired":          signInitData(testToken, launch(42, testNow.Add(-25*time.Hour), "")),
		"from the future":  signInitData(testToken, launch(42, testNow.Add(time.Hour), "")),
		"tampered user id": strings.Replace(signInitData(testToken, launch(42, testNow, "")), "42", "43", 1),
		"garbage":          "not-init-data",
	}
	for name, initData := range cases {
		t.Run(name, func(t *testing.T) {
			rec := do(h, "/api/v1/me", map[string]string{HeaderInitData: initData})
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401; body = %s", rec.Code, rec.Body)
			}
		})
	}
}

func TestDevModeHeader(t *testing.T) {
	headers := map[string]string{HeaderDevUserID: "7", HeaderDevStartParam: "demo-slug"}

	prod, _ := newTestServer(false)
	if rec := do(prod, "/api/v1/me", headers); rec.Code != http.StatusUnauthorized {
		t.Fatalf("dev header must be ignored without DEV_MODE, got %d", rec.Code)
	}

	dev, users := newTestServer(true)
	rec := do(dev, "/api/v1/me", headers)
	if rec.Code != http.StatusOK || !users.seen[7] {
		t.Fatalf("dev mode: status = %d, body = %s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), `"dev_mode":true`) {
		t.Fatalf("dev_mode flag expected: %s", rec.Body)
	}
}

func TestNoCredentials(t *testing.T) {
	h, _ := newTestServer(true)
	if rec := do(h, "/api/v1/me", nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestHouseNotFound(t *testing.T) {
	h, _ := newTestServer(true)
	rec := do(h, "/api/v1/houses/unknown", map[string]string{HeaderDevUserID: "7"})
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "house_not_found") {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
}

func TestHealthzIsPublic(t *testing.T) {
	h, _ := newTestServer(false)
	if rec := do(h, "/api/v1/healthz", nil); rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestReadyzIsPublic(t *testing.T) {
	h, _ := newTestServer(false)
	if rec := do(h, "/api/v1/readyz", nil); rec.Code != http.StatusOK {
		t.Fatalf("ready status = %d", rec.Code)
	}

	h, _ = newTestServerWithReadiness(false, errors.New("database unavailable"))
	if rec := do(h, "/api/v1/readyz", nil); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("not ready status = %d, want 503", rec.Code)
	}
}
