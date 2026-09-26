package httpapi

import (
	"context"
	"errors"
	"fmt"
	"io"
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

// Initiative endpoints of stage 1 (docs/API_DESCRIPTION.md, «Минимальный API сквозного
// MVP»): create from a template, start the poll, vote from the mini-app, watch the
// progress in м². A verified owner of the house creates and votes; the progress is
// visible to verified members and the management company (решения 19, 43).

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

// Voter casts poll votes (the poll module).
type Voter interface {
	CastVote(ctx context.Context, in poll.CastInput) (poll.CastResult, error)
}

// DemoMembership confirms an owner in a demo house (the access module).
type DemoMembership interface {
	ConfirmDemoOwner(ctx context.Context, userID, houseID, premiseNumber string, ownerIndex int) (access.OwnerLink, error)
}

// Poll duration chosen by the initiator: at least an hour, at most 30 days (решение 18).
const (
	minPollDuration = time.Hour
	maxPollDuration = 30 * 24 * time.Hour
)

func (h *handlers) createInitiative(c *gin.Context) {
	id, ok := IdentityFrom(c)
	if !ok {
		writeError(c, http.StatusInternalServerError, "internal", "Внутренняя ошибка, попробуйте ещё раз")

		return
	}
	houseID := c.Param("house")

	var body struct {
		TemplateCode string         `json:"template_code" binding:"required"`
		Title        string         `json:"title" binding:"required,max=200"`
		Description  string         `json:"description" binding:"max=2000"`
		Params       map[string]any `json:"params"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "Проверьте поля запроса")

		return
	}

	// Создать инициативу может подтверждённый собственник дома (решение 8).
	owner, err := h.access.IsVerifiedOwnerIn(c.Request.Context(), id.UserID, houseID)
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
		HouseID:         houseID,
		InitiatorUserID: id.UserID,
		TemplateCode:    body.TemplateCode,
		Title:           body.Title,
		Description:     body.Description,
		Params:          body.Params,
	})
	switch {
	case errors.Is(err, initiatives.ErrEmptyTitle):
		writeError(c, http.StatusBadRequest, "invalid_request", "Укажите название инициативы")
	case errors.Is(err, initiatives.ErrTooManyInitiatives):
		writeError(c, http.StatusTooManyRequests, "too_many_initiatives",
			fmt.Sprintf("За сутки можно создать не больше %d инициатив", initiatives.MaxInitiativesPerDay))
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
		writeJSON(c, http.StatusCreated, toInitiativeJSON(created, id.UserID))
	}
}

func (h *handlers) startPoll(c *gin.Context) {
	id, ok := IdentityFrom(c)
	if !ok {
		writeError(c, http.StatusInternalServerError, "internal", "Внутренняя ошибка, попробуйте ещё раз")

		return
	}

	// The body may be empty (the default 7 days), but a present one must be valid.
	var body struct {
		EndsAt *time.Time `json:"ends_at"`
	}
	if err := c.ShouldBindJSON(&body); err != nil && !errors.Is(err, io.EOF) {
		writeError(c, http.StatusBadRequest, "invalid_request", "Укажите окончание опроса в формате RFC 3339")

		return
	}
	var endsAt time.Time
	if body.EndsAt != nil {
		now := time.Now()
		if body.EndsAt.Before(now.Add(minPollDuration)) || body.EndsAt.After(now.Add(maxPollDuration)) {
			writeError(c, http.StatusBadRequest, "invalid_poll_duration", "Опрос может длиться от часа до 30 дней")

			return
		}
		endsAt = *body.EndsAt
	}

	started, err := h.pollStarter.StartPoll(c.Request.Context(), c.Param("id"), id.UserID, endsAt)
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
		writeJSON(c, http.StatusOK, toInitiativeJSON(started, id.UserID))
	}
}

// myVote is the vote from the mini-app; in the chat the same vote is cast by buttons.
func (h *handlers) myVote(c *gin.Context) {
	id, ok := IdentityFrom(c)
	if !ok {
		writeError(c, http.StatusInternalServerError, "internal", "Внутренняя ошибка, попробуйте ещё раз")

		return
	}

	var body struct {
		Choice          string  `json:"choice" binding:"required,oneof=for against"`
		OfficialChannel *string `json:"official_channel" binding:"omitempty,oneof=gosuslugi paper"`
		WillingToHelp   *bool   `json:"willing_to_help"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "Проверьте поля голоса")

		return
	}

	initiativeID := c.Param("id")
	if _, err := h.pollStarter.Get(c.Request.Context(), initiativeID); err != nil {
		h.writeInitiativeError(c, err, "get initiative for vote")

		return
	}

	// The survey is asked of «за» voters only (инвариант 7). Without the survey
	// fields the stored answers are kept.
	var survey *poll.Survey
	if body.Choice == poll.ChoiceFor && (body.OfficialChannel != nil || body.WillingToHelp != nil) {
		survey = &poll.Survey{}
		if body.OfficialChannel != nil {
			survey.OfficialChannel = *body.OfficialChannel
		}
		if body.WillingToHelp != nil {
			survey.WillingToHelp = *body.WillingToHelp
		}
	}

	result, err := h.votes.CastVote(c.Request.Context(), poll.CastInput{
		InitiativeID: initiativeID, UserID: id.UserID, Choice: body.Choice, Survey: survey,
	})
	switch {
	case errors.Is(err, poll.ErrNotOwner):
		writeError(c, http.StatusForbidden, "not_owner", "Голосуют только подтверждённые собственники дома")
	case errors.Is(err, poll.ErrPollClosed):
		writeError(c, http.StatusConflict, "poll_closed", "Опрос завершён")
	case errors.Is(err, poll.ErrNoWeight):
		writeError(c, http.StatusConflict, "not_in_snapshot",
			"Вашей записи нет в версии реестра этого опроса. Отправьте обращение «Данные неверны»")
	case err != nil:
		h.log.Error("cast vote", "err", err)
		writeError(c, http.StatusInternalServerError, "internal", "Не удалось учесть голос, попробуйте ещё раз")
	default:
		writeJSON(c, http.StatusOK, myVoteJSON{
			Choice:    result.Choice,
			WeightM2:  m2(new(big.Rat).SetFrac64(result.WeightNum, result.WeightDen*100)),
			Premises:  result.PremiseNumber,
			UpdatedAt: result.UpdatedAt,
		})
	}
}

func (h *handlers) pollProgressHandler(c *gin.Context) {
	id, ok := IdentityFrom(c)
	if !ok {
		writeError(c, http.StatusInternalServerError, "internal", "Внутренняя ошибка, попробуйте ещё раз")

		return
	}

	initiativeID := c.Param("id")
	initiative, err := h.pollStarter.Get(c.Request.Context(), initiativeID)
	if err != nil {
		h.writeInitiativeError(c, err, "get initiative for progress")

		return
	}

	// Прогресс видят подтверждённые жители дома (решение 19) и сотрудники УК (решение 43).
	allowed, err := h.access.MayViewInitiatives(c.Request.Context(), id.UserID, initiative.HouseID)
	if err != nil {
		h.log.Error("check access to progress", "err", err)
		writeError(c, http.StatusInternalServerError, "internal", "Не удалось загрузить опрос, попробуйте ещё раз")

		return
	}
	if !allowed {
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

func (h *handlers) writeInitiativeError(c *gin.Context, err error, what string) {
	if errors.Is(err, initiatives.ErrNotFound) {
		writeError(c, http.StatusNotFound, "initiative_not_found", "Инициатива не найдена")

		return
	}
	h.log.Error(what, "err", err)
	writeError(c, http.StatusInternalServerError, "internal", "Не удалось загрузить инициативу, попробуйте ещё раз")
}

// demoMembership is a shortcut of the demo house: the jury confirms itself as an owner
// of a flat in one step instead of the full onboarding (docs/04, решение 51).
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
	case errors.Is(err, access.ErrOwnerTaken):
		// Инвариант 9: the text of docs/04 for the second account.
		writeError(c, http.StatusConflict, "owner_taken",
			"Этот собственник уже подтверждён за другим аккаунтом. Выберите другого собственника квартиры или другую квартиру")
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
	ID              string           `json:"id"`
	HouseID         string           `json:"house_id"`
	Title           string           `json:"title"`
	Description     string           `json:"description"`
	Stage           string           `json:"stage"`
	PollEndsAt      *time.Time       `json:"poll_ends_at"`
	IsInitiator     bool             `json:"is_initiator"`
	RegistryVersion int              `json:"registry_version,omitempty"`
	AgendaItems     []agendaItemJSON `json:"agenda_items,omitempty"`
}

type agendaItemJSON struct {
	Position     int    `json:"position"`
	Text         string `json:"text"`
	MajorityRule string `json:"majority_rule"`
}

// toInitiativeJSON: is_initiator says whether the caller leads the initiative.
func toInitiativeJSON(in initiatives.Initiative, callerID string) initiativeJSON {
	j := initiativeJSON{
		ID: in.ID, HouseID: in.HouseID, Title: in.Title, Description: in.Description,
		Stage: in.Stage, PollEndsAt: in.PollEndsAt,
		IsInitiator:     in.InitiatorUserID != nil && *in.InitiatorUserID == callerID,
		RegistryVersion: in.RegistryVersion,
	}
	for _, item := range in.AgendaItems {
		j.AgendaItems = append(j.AgendaItems, agendaItemJSON{
			Position: item.Position, Text: item.Text, MajorityRule: item.MajorityRule,
		})
	}

	return j
}

type myVoteJSON struct {
	Choice    string    `json:"choice"`
	WeightM2  string    `json:"weight_m2"`
	Premises  string    `json:"premises"`
	UpdatedAt time.Time `json:"updated_at"`
}

// pollJSON: the fields of the contract first, then the extra ones for the dashboard.
type pollJSON struct {
	ForM2         string     `json:"for_m2"`
	AgainstM2     string     `json:"against_m2"`
	TotalM2       string     `json:"total_m2"`
	DemandM2      string     `json:"demand_m2"`
	DemandReached bool       `json:"demand_reached"`
	PollEndsAt    *time.Time `json:"poll_ends_at"`

	InitiativeID string         `json:"initiative_id"`
	Title        string         `json:"title"`
	Stage        string         `json:"stage"`
	ForPercent   string         `json:"for_percent"`
	VotesFor     int            `json:"votes_for"`
	VotesAgainst int            `json:"votes_against"`
	Thresholds   thresholdsJSON `json:"thresholds"`
}

var rat100 = big.NewRat(100, 1)

func toProgressJSON(in initiatives.Initiative, p poll.Progress) pollJSON {
	thresholds := p.Thresholds()
	percentFor := "0.0"
	if total := p.TotalM2(); total.Sign() > 0 {
		percent := new(big.Rat).Quo(p.ForM2(), total)
		percent.Mul(percent, rat100)
		percentFor = percent.FloatString(1)
	}

	return pollJSON{
		ForM2:         m2(p.ForM2()),
		AgainstM2:     m2(p.AgainstM2()),
		TotalM2:       m2(p.TotalM2()),
		DemandM2:      m2(thresholds.Demand),
		DemandReached: p.DemandReached(),
		PollEndsAt:    in.PollEndsAt,

		InitiativeID: p.InitiativeID,
		Title:        in.Title,
		Stage:        p.Stage,
		ForPercent:   percentFor,
		VotesFor:     p.VotesFor,
		VotesAgainst: p.VotesAgainst,
		Thresholds: thresholdsJSON{
			DemandM2:      m2(thresholds.Demand),
			QuorumAboveM2: m2(thresholds.QuorumAbove),
			TwoThirdsM2:   m2(thresholds.TwoThirds),
		},
	}
}
