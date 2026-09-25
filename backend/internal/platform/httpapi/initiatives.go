package httpapi

import (
	"context"
	"errors"
	"math/big"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"maxhackathon/backend/internal/access"
	"maxhackathon/backend/internal/initiatives"
	"maxhackathon/backend/internal/poll"
	"maxhackathon/backend/internal/registry"
	"maxhackathon/backend/internal/rules"
)

// Initiative endpoints of stage 1: create from a template, start the poll, watch
// the progress in м². Everything is available to a verified owner of the house;
// the progress is visible to any verified member (решение 19).

// InitiativeCreator creates initiatives (the initiatives module).
type InitiativeCreator interface {
	CreateFromTemplate(ctx context.Context, in initiatives.CreateInput) (initiatives.Initiative, error)
}

// PollStarter starts the support poll (the initiatives module).
type PollStarter interface {
	StartPoll(ctx context.Context, initiativeID, byUserID string, pollEndsAt time.Time) (initiatives.Initiative, error)
	Get(ctx context.Context, id string) (initiatives.Initiative, error)
}

// PollProgress reads the poll progress (the poll module).
type PollProgress interface {
	Progress(ctx context.Context, initiativeID string) (poll.Progress, error)
}

// DemoMembership confirms an owner in a demo house (the access module).
type DemoMembership interface {
	ConfirmDemoOwner(ctx context.Context, userID, houseID, premiseNumber string, ownerIndex int) (access.OwnerLink, error)
}

func (h *handlers) createInitiative(c *gin.Context) {
	id, ok := IdentityFrom(c)
	if !ok {
		writeError(c, http.StatusInternalServerError, "internal", "Внутренняя ошибка, попробуйте ещё раз")

		return
	}

	var body struct {
		HouseID      string `json:"house_id" binding:"required"`
		TemplateCode string `json:"template_code" binding:"required"`
		Title        string `json:"title" binding:"required,max=200"`
		Description  string `json:"description" binding:"max=2000"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "Проверьте поля запроса")

		return
	}

	// Создать инициативу может подтверждённый собственник дома (решение 8).
	owner, err := h.access.IsVerifiedOwnerIn(c.Request.Context(), id.UserID, body.HouseID)
	if err != nil {
		h.log.Error("check owner for initiative", "err", err)
		writeError(c, http.StatusInternalServerError, "internal", "Не удалось создать инициативу, попробуйте ещё раз")

		return
	}
	if !owner {
		writeError(c, http.StatusForbidden, "not_owner", "Создавать инициативы могут подтверждённые собственники дома")

		return
	}

	created, err := h.initiatives.CreateFromTemplate(c.Request.Context(), initiatives.CreateInput{
		HouseID:         body.HouseID,
		InitiatorUserID: id.UserID,
		TemplateCode:    body.TemplateCode,
		Title:           body.Title,
		Description:     body.Description,
	})
	switch {
	case errors.Is(err, rules.ErrTemplateNotFound):
		writeError(c, http.StatusNotFound, "template_not_found", "Шаблон не найден")
	case errors.Is(err, registry.ErrNotApplied):
		writeError(c, http.StatusConflict, "no_registry", "У дома ещё нет применённой версии реестра")
	case errors.Is(err, registry.ErrNotFound):
		writeError(c, http.StatusNotFound, "house_not_found", "Дом не найден")
	case err != nil:
		h.log.Error("create initiative", "err", err)
		writeError(c, http.StatusInternalServerError, "internal", "Не удалось создать инициативу, попробуйте ещё раз")
	default:
		writeJSON(c, http.StatusCreated, toInitiativeJSON(created))
	}
}

func (h *handlers) startPoll(c *gin.Context) {
	id, ok := IdentityFrom(c)
	if !ok {
		writeError(c, http.StatusInternalServerError, "internal", "Внутренняя ошибка, попробуйте ещё раз")

		return
	}

	initiativeID := c.Param("id")
	var body struct {
		Days int `json:"days" binding:"omitempty,min=1,max=30"`
	}
	_ = c.ShouldBindJSON(&body) // the body may be empty

	endsAt := time.Time{}
	if body.Days > 0 {
		endsAt = time.Now().Add(time.Duration(body.Days) * 24 * time.Hour)
	}

	started, err := h.pollStarter.StartPoll(c.Request.Context(), initiativeID, id.UserID, endsAt)
	switch {
	case errors.Is(err, initiatives.ErrNotFound):
		writeError(c, http.StatusNotFound, "initiative_not_found", "Инициатива не найдена")
	case errors.Is(err, initiatives.ErrNotInitiator):
		writeError(c, http.StatusForbidden, "not_initiator", "Запустить опрос может только автор инициативы")
	case errors.Is(err, initiatives.ErrWrongStage):
		writeError(c, http.StatusConflict, "wrong_stage", "Опрос уже запущен или этап пройден")
	case err != nil:
		h.log.Error("start poll", "err", err)
		writeError(c, http.StatusInternalServerError, "internal", "Не удалось запустить опрос, попробуйте ещё раз")
	default:
		writeJSON(c, http.StatusOK, toInitiativeJSON(started))
	}
}

func (h *handlers) initiativeProgress(c *gin.Context) {
	id, ok := IdentityFrom(c)
	if !ok {
		writeError(c, http.StatusInternalServerError, "internal", "Внутренняя ошибка, попробуйте ещё раз")

		return
	}

	initiativeID := c.Param("id")
	initiative, err := h.pollStarter.Get(c.Request.Context(), initiativeID)
	if errors.Is(err, initiatives.ErrNotFound) {
		writeError(c, http.StatusNotFound, "initiative_not_found", "Инициатива не найдена")

		return
	}
	if err != nil {
		h.log.Error("get initiative for progress", "err", err)
		writeError(c, http.StatusInternalServerError, "internal", "Не удалось загрузить опрос, попробуйте ещё раз")

		return
	}

	// Прогресс видят подтверждённые жители дома (решение 19).
	member, err := h.access.IsVerifiedMemberIn(c.Request.Context(), id.UserID, initiative.HouseID)
	if err != nil {
		h.log.Error("check member for progress", "err", err)
		writeError(c, http.StatusInternalServerError, "internal", "Не удалось загрузить опрос, попробуйте ещё раз")

		return
	}
	if !member {
		writeError(c, http.StatusForbidden, "not_member", "Прогресс видят подтверждённые жители дома")

		return
	}

	progress, err := h.pollProgress.Progress(c.Request.Context(), initiativeID)
	if err != nil {
		h.log.Error("poll progress", "err", err)
		writeError(c, http.StatusInternalServerError, "internal", "Не удалось загрузить опрос, попробуйте ещё раз")

		return
	}

	writeJSON(c, http.StatusOK, toProgressJSON(initiative, progress))
}

func (h *handlers) demoMembership(c *gin.Context) {
	id, ok := IdentityFrom(c)
	if !ok {
		writeError(c, http.StatusInternalServerError, "internal", "Внутренняя ошибка, попробуйте ещё раз")

		return
	}

	house, err := h.houses.HouseBySlug(c.Request.Context(), c.Param("house"))
	if errors.Is(err, registry.ErrNotFound) {
		writeError(c, http.StatusNotFound, "house_not_found", "Дом не найден. Проверьте ссылку от управляющей компании")

		return
	}
	if err != nil {
		h.log.Error("house by slug for demo membership", "err", err)
		writeError(c, http.StatusInternalServerError, "internal", "Не удалось подтвердить квартиру, попробуйте ещё раз")

		return
	}

	var body struct {
		PremiseNumber string `json:"premise_number" binding:"required"`
		OwnerIndex    int    `json:"owner_index" binding:"omitempty,min=1"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "Укажите номер квартиры")

		return
	}

	link, err := h.demoMembers.ConfirmDemoOwner(c.Request.Context(), id.UserID, house.ID, body.PremiseNumber, body.OwnerIndex)
	switch {
	case errors.Is(err, access.ErrNotDemo):
		writeError(c, http.StatusForbidden, "not_demo", "Быстрое подтверждение работает только в демо-доме")
	case errors.Is(err, access.ErrNotFound):
		writeError(c, http.StatusNotFound, "premise_not_found", "Квартира не найдена. Проверьте номер")
	case err != nil:
		h.log.Error("demo membership", "err", err)
		writeError(c, http.StatusInternalServerError, "internal", "Не удалось подтвердить квартиру, попробуйте ещё раз")
	default:
		writeJSON(c, http.StatusOK, gin.H{
			"membership_id": link.MembershipID,
			"premise":       link.PremiseNumber,
			"role":          "owner",
			"status":        "verified",
			"method":        "demo",
		})
	}
}

type initiativeJSON struct {
	ID          string     `json:"id"`
	HouseID     string     `json:"house_id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Stage       string     `json:"stage"`
	PollEndsAt  *time.Time `json:"poll_ends_at"`
	IsInitiator bool       `json:"is_initiator"`
}

func toInitiativeJSON(in initiatives.Initiative) initiativeJSON {
	return initiativeJSON{
		ID: in.ID, HouseID: in.HouseID, Title: in.Title, Description: in.Description,
		Stage: in.Stage, PollEndsAt: in.PollEndsAt,
		IsInitiator: in.InitiatorUserID != nil,
	}
}

type progressJSON struct {
	InitiativeID  string         `json:"initiative_id"`
	Title         string         `json:"title"`
	Stage         string         `json:"stage"`
	TotalAreaM2   string         `json:"total_area_m2"`
	ForAreaM2     string         `json:"for_area_m2"`
	AgainstAreaM2 string         `json:"against_area_m2"`
	ForPercent    string         `json:"for_percent"`
	VotesFor      int            `json:"votes_for"`
	VotesAgainst  int            `json:"votes_against"`
	Demand        demandJSON     `json:"demand"`
	Thresholds    thresholdsJSON `json:"thresholds"`
}

type demandJSON struct {
	AreaM2  string `json:"area_m2"`
	Reached bool   `json:"reached"`
}

var rat100 = big.NewRat(100, 1)

func toProgressJSON(in initiatives.Initiative, p poll.Progress) progressJSON {
	thresholds := p.Thresholds()
	percentFor := "0"
	if total := p.TotalM2(); total.Sign() > 0 {
		percent := p.ForM2().Quo(p.ForM2(), total)
		percent.Mul(percent, rat100)
		percentFor = percent.FloatString(1)
	}

	return progressJSON{
		InitiativeID:  p.InitiativeID,
		Title:         in.Title,
		Stage:         p.Stage,
		TotalAreaM2:   m2(p.TotalM2()),
		ForAreaM2:     m2(p.ForM2()),
		AgainstAreaM2: m2(p.AgainstM2()),
		ForPercent:    percentFor,
		VotesFor:      p.VotesFor,
		VotesAgainst:  p.VotesAgainst,
		Demand:        demandJSON{AreaM2: m2(thresholds.Demand), Reached: p.DemandReached()},
		Thresholds: thresholdsJSON{
			DemandM2:      m2(thresholds.Demand),
			QuorumAboveM2: m2(thresholds.QuorumAbove),
			TwoThirdsM2:   m2(thresholds.TwoThirds),
		},
	}
}
