package httpapi

import (
	"context"
	"encoding/json"
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
// MVP»): create from a template, list and open initiatives, start the poll, vote from
// the mini-app, watch the progress in м². A verified owner of the house creates and
// votes. The initiatives are visible to verified members and the management company
// (решения 19, 43), a draft only to those who lead it (решение 76).

// InitiativeCreator creates initiatives (the initiatives module).
type InitiativeCreator interface {
	CreateFromTemplate(ctx context.Context, in initiatives.CreateInput) (initiatives.Initiative, error)
}

// PollStarter starts the support poll (the initiatives module).
type PollStarter interface {
	StartPoll(ctx context.Context, initiativeID, byUserID string, pollEndsAt time.Time) (initiatives.Initiative, error)
	Get(ctx context.Context, id string) (initiatives.Initiative, error)
}

// InitiativeReader reads the list of a house and the card (the initiatives module).
type InitiativeReader interface {
	ListByHouse(ctx context.Context, houseID, viewerID string) ([]initiatives.Initiative, error)
	Details(ctx context.Context, id string) (initiatives.Initiative, error)
}

// PollProgress reads the poll progress (the poll module).
type PollProgress interface {
	Progress(ctx context.Context, initiativeID string) (poll.Progress, error)
}

// Voter casts poll votes and reads the caller's own vote (the poll module).
type Voter interface {
	CastVote(ctx context.Context, in poll.CastInput) (poll.CastResult, error)
	MyVote(ctx context.Context, initiativeID, userID string) (poll.MyVote, bool, error)
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
		TemplateCode string `json:"template_code" binding:"required"`
		Title        string `json:"title" binding:"required,max=200"`
		Description  string `json:"description" binding:"max=2000"`
		// Checked against the form of the template by the initiatives module.
		Params json.RawMessage `json:"params"`
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
	var paramsErr *rules.ParamsError
	switch {
	case errors.As(err, &paramsErr):
		writeFieldError(c, http.StatusBadRequest, "invalid_params", paramsMessage(paramsErr), paramsErr.Field)
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

	// Прогресс видят подтверждённые жители дома (решение 19) и сотрудники УК (решение 43).
	initiative, ok := h.visibleInitiative(c, id.UserID, c.Param("id"), h.pollStarter.Get)
	if !ok {
		return
	}

	progress, err := h.pollProgress.Progress(c.Request.Context(), initiative.ID)
	if err != nil {
		h.log.Error("poll progress", "err", err)
		writeError(c, http.StatusInternalServerError, "internal", "Не удалось загрузить опрос, попробуйте ещё раз")

		return
	}

	writeJSON(c, http.StatusOK, toProgressJSON(initiative, progress))
}

// listInitiatives is the list of a house: the verified members and the staff of its
// management company see it (решения 19, 43).
func (h *handlers) listInitiatives(c *gin.Context) {
	id, ok := IdentityFrom(c)
	if !ok {
		writeError(c, http.StatusInternalServerError, "internal", "Внутренняя ошибка, попробуйте ещё раз")

		return
	}
	houseID := c.Param("house")

	allowed, err := h.access.MayViewInitiatives(c.Request.Context(), id.UserID, houseID)
	if err != nil {
		h.log.Error("check access to initiatives", "err", err)
		writeError(c, http.StatusInternalServerError, "internal", "Не удалось загрузить инициативы, попробуйте ещё раз")

		return
	}
	if !allowed {
		writeError(c, http.StatusForbidden, "not_member", "Инициативы дома видят его подтверждённые жители")

		return
	}

	list, err := h.initiativeReader.ListByHouse(c.Request.Context(), houseID, id.UserID)
	if err != nil {
		h.log.Error("list initiatives", "err", err)
		writeError(c, http.StatusInternalServerError, "internal", "Не удалось загрузить инициативы, попробуйте ещё раз")

		return
	}

	resp := initiativesResponse{Initiatives: make([]initiativeListItemJSON, 0, len(list))}
	for _, in := range list {
		resp.Initiatives = append(resp.Initiatives, initiativeListItemJSON{
			ID: in.ID, Title: in.Title, Stage: in.Stage, Path: in.Path, PollEndsAt: in.PollEndsAt,
			CreatedAt: in.CreatedAt, IsInitiator: isInitiator(in, id.UserID),
		})
	}
	writeJSON(c, http.StatusOK, resp)
}

// initiativeCard is the card: the initiative, its agenda and thresholds, the caller's
// vote and the actions the caller may take now.
func (h *handlers) initiativeCard(c *gin.Context) {
	id, ok := IdentityFrom(c)
	if !ok {
		writeError(c, http.StatusInternalServerError, "internal", "Внутренняя ошибка, попробуйте ещё раз")

		return
	}

	in, ok := h.visibleInitiative(c, id.UserID, c.Param("id"), h.initiativeReader.Details)
	if !ok {
		return
	}

	owner, err := h.access.IsVerifiedOwnerIn(c.Request.Context(), id.UserID, in.HouseID)
	if err != nil {
		h.log.Error("check owner for card", "err", err)
		writeError(c, http.StatusInternalServerError, "internal", "Не удалось загрузить инициативу, попробуйте ещё раз")

		return
	}
	vote, voted, err := h.votes.MyVote(c.Request.Context(), in.ID, id.UserID)
	if err != nil {
		h.log.Error("my vote for card", "err", err)
		writeError(c, http.StatusInternalServerError, "internal", "Не удалось загрузить инициативу, попробуйте ещё раз")

		return
	}

	card := toCardJSON(in, id.UserID, in.Actions(initiatives.Viewer{UserID: id.UserID, Owner: owner}))
	if voted {
		card.MyVote = &myVoteCardJSON{
			Choice:        vote.Choice,
			WeightM2:      m2(new(big.Rat).SetFrac64(vote.WeightNum, vote.WeightDen*100)),
			WillingToHelp: vote.WillingToHelp,
			UpdatedAt:     vote.UpdatedAt,
		}
		if vote.OfficialChannel != "" {
			card.MyVote.OfficialChannel = &vote.OfficialChannel
		}
	}
	writeJSON(c, http.StatusOK, card)
}

// visibleInitiative loads the initiative and checks that the caller may see it: a
// draft only those who lead it (решение 76), any other stage also the verified
// members of the house and the staff of its company (решения 19, 43). When the caller
// may not, it writes the error and returns false.
func (h *handlers) visibleInitiative(c *gin.Context, userID, initiativeID string,
	load func(ctx context.Context, id string) (initiatives.Initiative, error),
) (initiatives.Initiative, bool) {
	in, err := load(c.Request.Context(), initiativeID)
	if err != nil {
		h.writeInitiativeError(c, err, "load initiative")

		return in, false
	}
	if in.IsLedBy(userID) {
		return in, true
	}
	if in.Stage == initiatives.StageDraft {
		// A draft of another user does not exist for the caller.
		writeError(c, http.StatusNotFound, "initiative_not_found", "Инициатива не найдена")

		return in, false
	}

	allowed, err := h.access.MayViewInitiatives(c.Request.Context(), userID, in.HouseID)
	if err != nil {
		h.log.Error("check access to initiative", "err", err)
		writeError(c, http.StatusInternalServerError, "internal", "Не удалось загрузить инициативу, попробуйте ещё раз")

		return in, false
	}
	if !allowed {
		writeError(c, http.StatusForbidden, "not_member", "Инициативы дома видят его подтверждённые жители")

		return in, false
	}

	return in, true
}

func (h *handlers) writeInitiativeError(c *gin.Context, err error, what string) {
	if errors.Is(err, initiatives.ErrNotFound) {
		writeError(c, http.StatusNotFound, "initiative_not_found", "Инициатива не найдена")

		return
	}
	h.log.Error(what, "err", err)
	writeError(c, http.StatusInternalServerError, "internal", "Не удалось загрузить инициативу, попробуйте ещё раз")
}

// paramsMessage explains to the user what is wrong with a field of the form.
func paramsMessage(e *rules.ParamsError) string {
	label := e.Title
	if label == "" {
		label = e.Field
	}

	switch e.Reason {
	case rules.ParamsNotObject:
		return "Поля формы переданы в неверном формате"
	case rules.ParamsUnknownField:
		return "В форме шаблона нет такого поля"
	case rules.ParamsRequired:
		return fmt.Sprintf("Заполните поле «%s»", label)
	case rules.ParamsEnum:
		return fmt.Sprintf("Выберите значение поля «%s» из списка", label)
	case rules.ParamsMinimum:
		return fmt.Sprintf("Поле «%s»: значение не меньше %s", label, e.Limit)
	case rules.ParamsMaximum:
		return fmt.Sprintf("Поле «%s»: значение не больше %s", label, e.Limit)
	case rules.ParamsMinLength:
		return fmt.Sprintf("Поле «%s»: длина не меньше %s", label, e.Limit)
	case rules.ParamsMaxLength:
		return fmt.Sprintf("Поле «%s»: длина не больше %s", label, e.Limit)
	default:
		return fmt.Sprintf("Проверьте значение поля «%s»", label)
	}
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
	Position       int    `json:"position"`
	Text           string `json:"text"`
	MajorityRule   string `json:"majority_rule"`
	LegalReference string `json:"legal_reference,omitempty"`
}

// toInitiativeJSON: is_initiator says whether the caller leads the initiative.
func toInitiativeJSON(in initiatives.Initiative, callerID string) initiativeJSON {
	return initiativeJSON{
		ID: in.ID, HouseID: in.HouseID, Title: in.Title, Description: in.Description,
		Stage: in.Stage, PollEndsAt: in.PollEndsAt,
		IsInitiator:     isInitiator(in, callerID),
		RegistryVersion: in.RegistryVersion,
		AgendaItems:     toAgendaJSON(in.AgendaItems),
	}
}

func isInitiator(in initiatives.Initiative, userID string) bool {
	return in.InitiatorUserID != nil && *in.InitiatorUserID == userID
}

func toAgendaJSON(items []initiatives.AgendaItem) []agendaItemJSON {
	if items == nil {
		return nil
	}
	result := make([]agendaItemJSON, 0, len(items))
	for _, item := range items {
		result = append(result, agendaItemJSON{
			Position: item.Position, Text: item.Text, MajorityRule: item.MajorityRule,
			LegalReference: item.LegalReference,
		})
	}

	return result
}

type initiativesResponse struct {
	Initiatives []initiativeListItemJSON `json:"initiatives"`
}

type initiativeListItemJSON struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	Stage       string     `json:"stage"`
	Path        *string    `json:"path"`
	PollEndsAt  *time.Time `json:"poll_ends_at"`
	CreatedAt   time.Time  `json:"created_at"`
	IsInitiator bool       `json:"is_initiator"`
}

// initiativeCardJSON is GET /initiatives/{id}. Areas are decimal strings in м², the
// thresholds come from the registry snapshot of the initiative (решение 55).
type initiativeCardJSON struct {
	ID              string           `json:"id"`
	HouseID         string           `json:"house_id"`
	Title           string           `json:"title"`
	Description     string           `json:"description"`
	Stage           string           `json:"stage"`
	Path            *string          `json:"path"`
	PollEndsAt      *time.Time       `json:"poll_ends_at"`
	CreatedAt       time.Time        `json:"created_at"`
	IsInitiator     bool             `json:"is_initiator"`
	Template        *templateRefJSON `json:"template"`
	Params          json.RawMessage  `json:"params"`
	RegistryVersion int              `json:"registry_version"`
	TotalAreaM2     string           `json:"total_area_m2"`
	Thresholds      thresholdsJSON   `json:"thresholds"`
	AgendaItems     []agendaItemJSON `json:"agenda_items"`
	MyVote          *myVoteCardJSON  `json:"my_vote"`
	AllowedActions  []actionJSON     `json:"allowed_actions"`
}

type templateRefJSON struct {
	Code    string `json:"code"`
	Name    string `json:"name"`
	Version int    `json:"version"`
}

// myVoteCardJSON is the caller's vote in the poll; the survey is asked of «за» only.
type myVoteCardJSON struct {
	Choice          string    `json:"choice"`
	WeightM2        string    `json:"weight_m2"`
	OfficialChannel *string   `json:"official_channel"`
	WillingToHelp   bool      `json:"willing_to_help"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type actionJSON struct {
	Code       string `json:"code"`
	Allowed    bool   `json:"allowed"`
	ReasonCode string `json:"reason_code,omitempty"`
}

func toCardJSON(in initiatives.Initiative, callerID string, actions []initiatives.Action) initiativeCardJSON {
	total := registry.CentiToM2(in.TotalAreaCenti)
	t := rules.ForTotal(total)
	card := initiativeCardJSON{
		ID: in.ID, HouseID: in.HouseID, Title: in.Title, Description: in.Description,
		Stage: in.Stage, Path: in.Path, PollEndsAt: in.PollEndsAt, CreatedAt: in.CreatedAt,
		IsInitiator:     isInitiator(in, callerID),
		Params:          in.Params,
		RegistryVersion: in.RegistryVersion,
		TotalAreaM2:     m2(total),
		Thresholds: thresholdsJSON{
			DemandM2: m2(t.Demand), QuorumAboveM2: m2(t.QuorumAbove), TwoThirdsM2: m2(t.TwoThirds),
		},
		AgendaItems:    toAgendaJSON(in.AgendaItems),
		AllowedActions: make([]actionJSON, 0, len(actions)),
	}
	if len(card.Params) == 0 {
		card.Params = json.RawMessage(`{}`)
	}
	if card.AgendaItems == nil {
		card.AgendaItems = []agendaItemJSON{}
	}
	if in.Template != nil {
		card.Template = &templateRefJSON{Code: in.Template.Code, Name: in.Template.Name, Version: in.Template.Version}
	}
	for _, a := range actions {
		card.AllowedActions = append(card.AllowedActions, actionJSON{Code: a.Code, Allowed: a.Allowed, ReasonCode: a.Reason})
	}

	return card
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
