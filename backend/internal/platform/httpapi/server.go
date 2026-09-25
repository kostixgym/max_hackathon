// Package httpapi is the HTTP API of the mini-app (an inbound adapter): it checks
// who is calling and translates HTTP to calls of the domain modules.
package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"math/big"
	"net/http"
	"time"

	"maxhackathon/backend/internal/registry"
	"maxhackathon/backend/internal/rules"
)

// Houses reads house data (the registry module).
type Houses interface {
	HouseBySlug(ctx context.Context, slug string) (registry.HouseSummary, error)
}

// Deps are the dependencies of the API.
type Deps struct {
	Auth    *Authenticator
	Houses  Houses
	Log     *slog.Logger
	DevMode bool
}

// NewHandler builds the router of the API.
func NewHandler(d Deps) http.Handler {
	h := &handlers{houses: d.Houses, log: d.Log, devMode: d.DevMode}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", h.healthz)
	mux.Handle("GET /api/me", d.Auth.Middleware(http.HandlerFunc(h.me)))
	mux.Handle("GET /api/houses/{slug}", d.Auth.Middleware(http.HandlerFunc(h.house)))

	return recoverer(d.Log, logRequests(d.Log, mux))
}

type handlers struct {
	houses  Houses
	log     *slog.Logger
	devMode bool
}

func (h *handlers) healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type meResponse struct {
	User    meUser     `json:"user"`
	DevMode bool       `json:"dev_mode"`
	House   *houseJSON `json:"house"`
}

type meUser struct {
	ID        string `json:"id"`
	FirstName string `json:"first_name"`
}

// me returns the caller and, if the mini-app was opened by a house link
// (start_param = invite slug), the summary of that house.
func (h *handlers) me(w http.ResponseWriter, r *http.Request) {
	id, ok := IdentityFrom(r.Context())
	if !ok {
		// Only possible if the route is registered without the auth middleware.
		h.log.Error("me: no identity in context")
		writeError(w, http.StatusInternalServerError, "internal", "Внутренняя ошибка, попробуйте ещё раз")

		return
	}
	resp := meResponse{
		User:    meUser{ID: id.UserID, FirstName: id.FirstName},
		DevMode: h.devMode,
	}

	if id.StartParam != "" {
		house, err := h.houses.HouseBySlug(r.Context(), id.StartParam)
		switch {
		case err == nil:
			j := toHouseJSON(house)
			resp.House = &j
		case !errors.Is(err, registry.ErrNotFound):
			h.log.Error("house by start_param", "err", err)
			writeError(w, http.StatusInternalServerError, "internal", "Не удалось загрузить дом, попробуйте ещё раз")

			return
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

func (h *handlers) house(w http.ResponseWriter, r *http.Request) {
	house, err := h.houses.HouseBySlug(r.Context(), r.PathValue("slug"))
	switch {
	case errors.Is(err, registry.ErrNotFound):
		writeError(w, http.StatusNotFound, "house_not_found", "Дом не найден. Проверьте ссылку от управляющей компании")

		return
	case err != nil:
		h.log.Error("house by slug", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "Не удалось загрузить дом, попробуйте ещё раз")

		return
	}

	writeJSON(w, http.StatusOK, toHouseJSON(house))
}

// houseJSON: areas are decimal strings in м² with a dot ("3000.00"): exact values
// travel as text, the mini-app formats them for display.
type houseJSON struct {
	Slug            string          `json:"slug"`
	Address         string          `json:"address"`
	Region          string          `json:"region"`
	IsDemo          bool            `json:"is_demo"`
	PremisesCount   int             `json:"premises_count"`
	RegistryVersion *int            `json:"registry_version"`
	TotalAreaM2     *string         `json:"total_area_m2"`
	Thresholds      *thresholdsJSON `json:"thresholds"`
}

type thresholdsJSON struct {
	DemandM2      string `json:"demand_m2"`
	QuorumAboveM2 string `json:"quorum_above_m2"`
	TwoThirdsM2   string `json:"two_thirds_m2"`
}

func toHouseJSON(h registry.HouseSummary) houseJSON {
	j := houseJSON{
		Slug:            h.InviteSlug,
		Address:         h.Address,
		Region:          h.Region,
		IsDemo:          h.IsDemo,
		PremisesCount:   h.PremisesCount,
		RegistryVersion: h.RegistryVersion,
	}

	if h.TotalAreaCenti != nil {
		total := registry.CentiToM2(*h.TotalAreaCenti)
		t := rules.ForTotal(total)
		s := m2(total)
		j.TotalAreaM2 = &s
		j.Thresholds = &thresholdsJSON{
			DemandM2:      m2(t.Demand),
			QuorumAboveM2: m2(t.QuorumAbove),
			TwoThirdsM2:   m2(t.TwoThirds),
		}
	}

	return j
}

func m2(v *big.Rat) string { return v.FloatString(2) }

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (s *statusWriter) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// logRequests logs method, path, status and duration. Headers are not logged:
// initData contains personal data of the user.
func logRequests(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		log.Info("http", "method", r.Method, "path", r.URL.Path, "status", sw.status, "duration_ms", time.Since(start).Milliseconds())
	})
}

func recoverer(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				log.Error("panic in handler", "panic", v, "path", r.URL.Path)
				writeError(w, http.StatusInternalServerError, "internal", "Внутренняя ошибка, попробуйте ещё раз")
			}
		}()
		next.ServeHTTP(w, r)
	})
}
