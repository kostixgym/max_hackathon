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

	"github.com/gin-gonic/gin"

	"maxhackathon/backend/internal/registry"
	"maxhackathon/backend/internal/rules"
)

// Houses reads house data (the registry module).
type Houses interface {
	HouseBySlug(ctx context.Context, slug string) (registry.HouseSummary, error)
}

// Readiness checks whether a required dependency is available.
type Readiness interface {
	Ping(ctx context.Context) error
}

// AccessChecks answers permission questions of the endpoints (the access module).
type AccessChecks interface {
	IsVerifiedOwnerIn(ctx context.Context, userID, houseID string) (bool, error)
	MayViewInitiatives(ctx context.Context, userID, houseID string) (bool, error)
}

// Deps are the dependencies of the API.
type Deps struct {
	Auth     *Authenticator
	Houses   Houses
	Profiles Profiles
	DB       Readiness
	Log      *slog.Logger
	DevMode  bool

	// Stage 1: templates, initiatives and the support poll.
	Access           AccessChecks
	Templates        TemplateCatalog
	Initiatives      InitiativeCreator
	InitiativeReader InitiativeReader
	PollStarter      PollStarter
	PollProgress     PollProgress
	Votes            Voter
	DemoMembers      DemoMembership

	// Sprint to 30.09: the staff cabinet (К2), demands (Дима, Д1–Д5) and meetings
	// (Гоша, Г2–Г7). The Demands and Meetings interfaces live in demands.go and
	// meetings.go: their owners add methods there and the wiring in main.go, this
	// file stays as it is. A nil module keeps its routes at 501.
	Orgs      Orgs
	OrgHouses OrgHouses
	Demands   Demands
	Meetings  Meetings
}

// maxBodyBytes limits a request body: the largest one, an initiative with its
// parameters, is a few kilobytes; without a limit a client could make the server
// read megabytes into memory.
const maxBodyBytes = 64 << 10

// NewHandler builds the router of the API.
func NewHandler(d Deps) http.Handler {
	h := &handlers{
		houses: d.Houses, profiles: d.Profiles, db: d.DB, log: d.Log, devMode: d.DevMode,
		access: d.Access, templates: d.Templates, initiatives: d.Initiatives,
		initiativeReader: d.InitiativeReader, pollStarter: d.PollStarter,
		pollProgress: d.PollProgress, votes: d.Votes, demoMembers: d.DemoMembers,
		orgs: d.Orgs, orgHouses: d.OrgHouses, demands: d.Demands, meetings: d.Meetings,
	}

	// Application logs are emitted through slog; Gin's debug route dump would
	// otherwise mix plain text into the JSON log stream.
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.HandleMethodNotAllowed = true
	router.Use(requestLogger(d.Log), recovery(d.Log), limitBody(maxBodyBytes))
	router.NoRoute(func(c *gin.Context) {
		writeError(c, http.StatusNotFound, "not_found", "Маршрут не найден")
	})
	router.NoMethod(func(c *gin.Context) {
		writeError(c, http.StatusMethodNotAllowed, "method_not_allowed", "Метод не поддерживается")
	})

	api := router.Group("/api/v1")
	api.GET("/healthz", h.healthz)
	api.GET("/readyz", h.readyz)

	protected := api.Group("")
	protected.Use(d.Auth.Middleware())
	protected.GET("/me", h.me)
	protected.GET("/houses/:house", h.house)
	protected.GET("/premises/:premiseID/owners", h.premiseOwners)
	protected.GET("/houses/:house/meeting-officer-candidates", h.meetingOfficerCandidates)

	// Stage 1: the support poll. :house is the invite slug for the demo shortcut (it
	// comes from the house link) and the house id for initiatives (it comes from /me).
	protected.GET("/templates", h.listTemplates)
	protected.GET("/templates/:code", h.template)
	protected.POST("/houses/:house/demo-membership", h.demoMembership)
	protected.GET("/houses/:house/initiatives", h.listInitiatives)
	protected.POST("/houses/:house/initiatives", h.createInitiative)
	protected.GET("/initiatives/:id", h.initiativeCard)
	protected.POST("/initiatives/:id/start-poll", h.startPoll)
	protected.PUT("/initiatives/:id/my-vote", h.myVote)
	protected.GET("/initiatives/:id/poll", h.pollProgressHandler)

	// Sprint to 30.09 (К0): the routes of path A are final, the handlers of
	// demand and meeting modules land with their steps (Д1–Д3, Г2–Г6) and answer
	// 501 not_implemented until then.
	protected.POST("/houses/:house/demo-staff", h.demoStaff)
	protected.GET("/orgs", h.myOrgs)
	protected.GET("/orgs/:orgID/houses", h.orgHousesList)
	protected.GET("/orgs/:orgID/demands", h.orgDemands)
	protected.POST("/initiatives/:id/demand", h.createDemand)
	protected.GET("/demands/:id", h.getDemand)
	protected.POST("/demands/:id/mark-delivered", h.markDemandDelivered)
	protected.GET("/demands/:id/pdf", h.demandPDF)
	protected.POST("/initiatives/:id/meetings", h.createMeeting)
	protected.GET("/meetings/:id", h.getMeeting)
	protected.GET("/meetings/:id/tracker", h.meetingTracker)
	protected.POST("/meetings/:id/ballots/receive", h.receiveBallot)
	protected.PUT("/ballots/:id/decisions", h.ballotDecisions)
	protected.GET("/meetings/:id/result-preview", h.meetingResultPreview)
	protected.POST("/meetings/:id/finalize", h.finalizeMeeting)
	protected.GET("/meetings/:id/protocol.pdf", h.meetingProtocolPDF)
	protected.POST("/meetings/:id/demo/finish-voting", h.demoFinishVoting)
	protected.POST("/meetings/:id/demo/fill-ballots", h.demoFillBallots)

	return router
}

// limitBody rejects a body larger than n bytes: at once by Content-Length, and while
// reading for a body without it.
func limitBody(n int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.ContentLength > n {
			writeError(c, http.StatusRequestEntityTooLarge, "request_too_large", "Слишком большой запрос")
			c.Abort()

			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, n)
		c.Next()
	}
}

type handlers struct {
	houses   Houses
	profiles Profiles
	db       Readiness
	log      *slog.Logger
	devMode  bool

	// Stage 1.
	access           AccessChecks
	templates        TemplateCatalog
	initiatives      InitiativeCreator
	initiativeReader InitiativeReader
	pollStarter      PollStarter
	pollProgress     PollProgress
	votes            Voter
	demoMembers      DemoMembership

	// Sprint to 30.09.
	orgs      Orgs
	orgHouses OrgHouses
	demands   Demands
	meetings  Meetings
}

func (h *handlers) healthz(c *gin.Context) {
	writeJSON(c, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *handlers) readyz(c *gin.Context) {
	if h.db == nil {
		writeError(c, http.StatusServiceUnavailable, "not_ready", "Сервис временно не готов")

		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	if err := h.db.Ping(ctx); err != nil {
		h.log.Warn("readiness check failed", "err", err)
		writeError(c, http.StatusServiceUnavailable, "not_ready", "Сервис временно не готов")

		return
	}

	writeJSON(c, http.StatusOK, map[string]string{"status": "ready"})
}

type meResponse struct {
	User        meUser           `json:"user"`
	DevMode     bool             `json:"dev_mode"`
	House       *houseJSON       `json:"house"`
	Memberships []membershipJSON `json:"memberships"`
	Orgs        []orgJSON        `json:"orgs"`
}

type meUser struct {
	ID        string `json:"id"`
	FirstName string `json:"first_name"`
}

// me returns the caller and, if the mini-app was opened by a house link
// (start_param = invite slug), the summary of that house.
func (h *handlers) me(c *gin.Context) {
	id, ok := IdentityFrom(c)
	if !ok {
		// Only possible if the route is registered without the auth middleware.
		h.log.Error("me: no identity in context")
		writeError(c, http.StatusInternalServerError, "internal", "Внутренняя ошибка, попробуйте ещё раз")

		return
	}
	resp := meResponse{
		User:        meUser{ID: id.UserID, FirstName: id.FirstName},
		DevMode:     h.devMode,
		Memberships: make([]membershipJSON, 0),
		Orgs:        make([]orgJSON, 0),
	}

	memberships, err := h.profiles.MembershipsByUser(c.Request.Context(), id.UserID)
	if err != nil {
		h.log.Error("list my memberships", "err", err)
		writeError(c, http.StatusInternalServerError, "internal", "Не удалось загрузить привязки, попробуйте ещё раз")

		return
	}
	for _, membership := range memberships {
		resp.Memberships = append(resp.Memberships, toMembershipJSON(membership))
	}

	// The «Жилец / УК» switch: organizations the user works in (К2).
	if h.orgs != nil {
		orgs, err := h.orgs.OrgsByUser(c.Request.Context(), id.UserID)
		if err != nil {
			h.log.Error("list my orgs", "err", err)
			writeError(c, http.StatusInternalServerError, "internal", "Не удалось загрузить данные, попробуйте ещё раз")

			return
		}
		resp.Orgs = toOrgsJSON(orgs)
	}

	if id.StartParam != "" {
		house, err := h.houses.HouseBySlug(c.Request.Context(), id.StartParam)
		switch {
		case err == nil:
			j := toHouseJSON(house)
			resp.House = &j
		case !errors.Is(err, registry.ErrNotFound):
			h.log.Error("house by start_param", "err", err)
			writeError(c, http.StatusInternalServerError, "internal", "Не удалось загрузить дом, попробуйте ещё раз")

			return
		}
	}

	writeJSON(c, http.StatusOK, resp)
}

func (h *handlers) house(c *gin.Context) {
	house, err := h.houses.HouseBySlug(c.Request.Context(), c.Param("house"))
	switch {
	case errors.Is(err, registry.ErrNotFound):
		writeError(c, http.StatusNotFound, "house_not_found", "Дом не найден. Проверьте ссылку от управляющей компании")

		return
	case err != nil:
		h.log.Error("house by slug", "err", err)
		writeError(c, http.StatusInternalServerError, "internal", "Не удалось загрузить дом, попробуйте ещё раз")

		return
	}

	writeJSON(c, http.StatusOK, toHouseJSON(house))
}

// houseJSON: areas are decimal strings in м² with a dot ("3000.00"): exact values
// travel as text, the mini-app formats them for display.
type houseJSON struct {
	ID              string          `json:"id"`
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
		ID:              h.ID,
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

// logRequests logs method, path, status and duration. Headers are not logged:
// initData contains personal data of the user.
func requestLogger(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		log.Info("http", "method", c.Request.Method, "path", c.Request.URL.Path,
			"status", c.Writer.Status(), "duration_ms", time.Since(start).Milliseconds())
	}
}

func recovery(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if v := recover(); v != nil {
				log.Error("panic in handler", "panic", v, "path", c.Request.URL.Path)
				if !c.Writer.Written() {
					writeError(c, http.StatusInternalServerError, "internal", "Внутренняя ошибка, попробуйте ещё раз")
				}
				c.Abort()
			}
		}()
		c.Next()
	}
}
