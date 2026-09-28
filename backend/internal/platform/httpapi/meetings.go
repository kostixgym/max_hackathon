package httpapi

// Meeting endpoints (docs/plan-do-30-09.md, Г2–Г6; the contract is the «Собрание»
// section of docs/API_DESCRIPTION.md). Who may act is decided inside the module by
// access.ManagesAsStaff: in the demo house a tester runs only the meetings of their
// own initiatives (решение 79). The protocol PDF is Дима's (documents.go).

import (
	"context"
	"errors"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"maxhackathon/backend/internal/meeting"
)

// Meetings is the meeting module (nil keeps the routes at 501).
type Meetings interface {
	Create(ctx context.Context, in meeting.CreateInput) (meeting.View, error)
	ActiveMeeting(ctx context.Context, initiativeID string) (meeting.Ref, bool, error)
	Get(ctx context.Context, meetingID, viewerID string) (meeting.View, error)
	Tracker(ctx context.Context, meetingID, viewerID string) (meeting.Tracker, error)
	ReceiveBallot(ctx context.Context, meetingID, ballotID, byUserID string) (meeting.ReceivedBallot, error)
	RecordDecisions(ctx context.Context, ballotID, byUserID string, decisions []meeting.Decision) (meeting.BallotDecisions, error)
	RecordGISResults(ctx context.Context, meetingID, byUserID string, input meeting.GISResults) (meeting.GISResults, error)
	Preview(ctx context.Context, meetingID, viewerID string) (meeting.Result, error)
	Finalize(ctx context.Context, meetingID, byUserID string) (meeting.Final, error)
	ProtocolData(ctx context.Context, meetingID string) (meeting.Protocol, error)
	FinishVoting(ctx context.Context, meetingID, byUserID string) (meeting.View, error)
	FillBallots(ctx context.Context, meetingID, byUserID string) (meeting.View, error)
}

func (h *handlers) getInitiativeMeeting(c *gin.Context) {
	if h.meetings == nil {
		notImplemented(c, "Собрания временно недоступны")
		return
	}
	id, ok := IdentityFrom(c)
	if !ok {
		writeError(c, http.StatusInternalServerError, "internal", "Внутренняя ошибка, попробуйте ещё раз")
		return
	}
	ref, found, err := h.meetings.ActiveMeeting(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.writeMeetingError(c, err, "find initiative meeting")
		return
	}
	if !found {
		writeError(c, http.StatusNotFound, "meeting_not_found", "Собрание не найдено")
		return
	}
	view, err := h.meetings.Get(c.Request.Context(), ref.ID, id.UserID)
	if err != nil {
		h.writeMeetingError(c, err, "read initiative meeting")
		return
	}
	writeJSON(c, http.StatusOK, toMeetingJSON(view))
}

func (h *handlers) createMeeting(c *gin.Context) {
	if h.meetings == nil {
		notImplemented(c, "Собрание появится вместе с модулем собрания (Г2)")

		return
	}
	id, ok := IdentityFrom(c)
	if !ok {
		writeError(c, http.StatusInternalServerError, "internal", "Внутренняя ошибка, попробуйте ещё раз")

		return
	}

	var body struct {
		Path             string     `json:"path" binding:"omitempty,oneof=A B"`
		Form             string     `json:"form" binding:"required,oneof=gis_electronic paper_absentee"`
		NoticeAt         *time.Time `json:"notice_at" binding:"required"`
		VotingStartsAt   *time.Time `json:"voting_starts_at" binding:"required"`
		VotingEndsAt     *time.Time `json:"voting_ends_at" binding:"required"`
		ChairOwnerID     string     `json:"chair_owner_id" binding:"required"`
		SecretaryOwnerID string     `json:"secretary_owner_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request",
			"Укажите форму собрания, даты в формате RFC 3339, председателя и секретаря")

		return
	}

	view, err := h.meetings.Create(c.Request.Context(), meeting.CreateInput{
		InitiativeID:     c.Param("id"),
		ByUserID:         id.UserID,
		Path:             body.Path,
		Form:             body.Form,
		NoticeAt:         *body.NoticeAt,
		VotingStartsAt:   *body.VotingStartsAt,
		VotingEndsAt:     *body.VotingEndsAt,
		ChairOwnerID:     body.ChairOwnerID,
		SecretaryOwnerID: body.SecretaryOwnerID,
	})
	if err != nil {
		h.writeMeetingError(c, err, "create meeting")

		return
	}

	writeJSON(c, http.StatusCreated, toMeetingJSON(view))
}

func (h *handlers) getMeeting(c *gin.Context) {
	if h.meetings == nil {
		notImplemented(c, "Собрание появится вместе с модулем собрания (Г3)")

		return
	}
	id, ok := IdentityFrom(c)
	if !ok {
		writeError(c, http.StatusInternalServerError, "internal", "Внутренняя ошибка, попробуйте ещё раз")

		return
	}

	view, err := h.meetings.Get(c.Request.Context(), c.Param("id"), id.UserID)
	if err != nil {
		h.writeMeetingError(c, err, "get meeting")

		return
	}

	writeJSON(c, http.StatusOK, toMeetingJSON(view))
}

func (h *handlers) meetingTracker(c *gin.Context) {
	if h.meetings == nil {
		notImplemented(c, "Трекер появится вместе с модулем собрания (Г3)")

		return
	}
	id, ok := IdentityFrom(c)
	if !ok {
		writeError(c, http.StatusInternalServerError, "internal", "Внутренняя ошибка, попробуйте ещё раз")

		return
	}

	tracker, err := h.meetings.Tracker(c.Request.Context(), c.Param("id"), id.UserID)
	if err != nil {
		h.writeMeetingError(c, err, "meeting tracker")

		return
	}

	writeJSON(c, http.StatusOK, toTrackerJSON(tracker))
}

func (h *handlers) receiveBallot(c *gin.Context) {
	if h.meetings == nil {
		notImplemented(c, "Отметка бюллетеней появится вместе с модулем собрания (Г3)")

		return
	}
	id, ok := IdentityFrom(c)
	if !ok {
		writeError(c, http.StatusInternalServerError, "internal", "Внутренняя ошибка, попробуйте ещё раз")

		return
	}

	var body struct {
		BallotID string `json:"ballot_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "Укажите бюллетень")

		return
	}

	received, err := h.meetings.ReceiveBallot(c.Request.Context(), c.Param("id"), body.BallotID, id.UserID)
	if err != nil {
		h.writeMeetingError(c, err, "receive ballot")

		return
	}

	writeJSON(c, http.StatusOK, receivedBallotJSON{
		BallotID: received.BallotID, Status: received.Status, ReceivedAt: received.ReceivedAt,
	})
}

func (h *handlers) ballotDecisions(c *gin.Context) {
	if h.meetings == nil {
		notImplemented(c, "Внесение решений появится вместе с модулем собрания (Г4)")

		return
	}
	id, ok := IdentityFrom(c)
	if !ok {
		writeError(c, http.StatusInternalServerError, "internal", "Внутренняя ошибка, попробуйте ещё раз")

		return
	}

	var body struct {
		Decisions []struct {
			AgendaItemID string `json:"agenda_item_id" binding:"required"`
			Choice       string `json:"choice" binding:"required,oneof=for against abstain"`
		} `json:"decisions" binding:"required,min=1,dive"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", decisionsMessage)

		return
	}
	decisions := make([]meeting.Decision, 0, len(body.Decisions))
	for _, d := range body.Decisions {
		decisions = append(decisions, meeting.Decision{AgendaItemID: d.AgendaItemID, Choice: d.Choice})
	}

	counted, err := h.meetings.RecordDecisions(c.Request.Context(), c.Param("id"), id.UserID, decisions)
	if err != nil {
		h.writeMeetingError(c, err, "record ballot decisions")

		return
	}

	j := ballotDecisionsJSON{BallotID: counted.BallotID, Status: counted.Status,
		Decisions: make([]decisionJSON, 0, len(counted.Decisions))}
	for _, d := range counted.Decisions {
		j.Decisions = append(j.Decisions, decisionJSON{AgendaItemID: d.AgendaItemID, Choice: d.Choice})
	}
	writeJSON(c, http.StatusOK, j)
}

func (h *handlers) meetingGISResults(c *gin.Context) {
	if h.meetings == nil {
		notImplemented(c, "Результаты ГИС ЖКХ появятся вместе с модулем собрания (Г8)")

		return
	}
	id, ok := IdentityFrom(c)
	if !ok {
		writeError(c, http.StatusInternalServerError, "internal", "Внутренняя ошибка, попробуйте ещё раз")

		return
	}

	var body gisResultsJSON
	if err := c.ShouldBindJSON(&body); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", gisResultsMessage)

		return
	}
	input := meeting.GISResults{Entries: make([]meeting.GISResultEntry, 0, len(body.Entries))}
	var valid bool
	input.OnlineParticipantsM2, valid = parseM2Input(body.OnlineParticipantsM2)
	if !valid {
		writeError(c, http.StatusBadRequest, "invalid_request", gisResultsMessage)

		return
	}
	for _, entry := range body.Entries {
		forM2, forOK := parseM2Input(entry.ForM2)
		againstM2, againstOK := parseM2Input(entry.AgainstM2)
		abstainM2, abstainOK := parseM2Input(entry.AbstainM2)
		if !forOK || !againstOK || !abstainOK {
			writeError(c, http.StatusBadRequest, "invalid_request", gisResultsMessage)

			return
		}
		input.Entries = append(input.Entries, meeting.GISResultEntry{
			AgendaItemID: entry.AgendaItemID,
			ForM2:        forM2,
			AgainstM2:    againstM2,
			AbstainM2:    abstainM2,
		})
	}

	saved, err := h.meetings.RecordGISResults(c.Request.Context(), c.Param("id"), id.UserID, input)
	if err != nil {
		h.writeMeetingError(c, err, "record GIS results")

		return
	}

	writeJSON(c, http.StatusOK, toGISResultsJSON(saved))
}

func (h *handlers) meetingResultPreview(c *gin.Context) {
	if h.meetings == nil {
		notImplemented(c, "Предпросмотр итога появится вместе с модулем собрания (Г5)")

		return
	}
	id, ok := IdentityFrom(c)
	if !ok {
		writeError(c, http.StatusInternalServerError, "internal", "Внутренняя ошибка, попробуйте ещё раз")

		return
	}

	result, err := h.meetings.Preview(c.Request.Context(), c.Param("id"), id.UserID)
	if err != nil {
		h.writeMeetingError(c, err, "meeting result preview")

		return
	}

	writeJSON(c, http.StatusOK, toResultJSON(result))
}

func (h *handlers) finalizeMeeting(c *gin.Context) {
	if h.meetings == nil {
		notImplemented(c, "Фиксация итога появится вместе с модулем собрания (Г5)")

		return
	}
	id, ok := IdentityFrom(c)
	if !ok {
		writeError(c, http.StatusInternalServerError, "internal", "Внутренняя ошибка, попробуйте ещё раз")

		return
	}

	final, err := h.meetings.Finalize(c.Request.Context(), c.Param("id"), id.UserID)
	if err != nil {
		h.writeMeetingError(c, err, "finalize meeting")

		return
	}

	result := toResultJSON(final.Result)
	writeJSON(c, http.StatusOK, finalJSON{
		MeetingID: final.MeetingID, Outcome: final.Outcome, FinalizedAt: final.FinalizedAt,
		ParticipantsM2: result.ParticipantsM2, TotalM2: result.TotalM2, QuorumReached: result.QuorumReached,
		Results: result.AgendaResults,
	})
}

func (h *handlers) demoFinishVoting(c *gin.Context) {
	if h.meetings == nil {
		notImplemented(c, "Демо-ускорители появятся вместе с модулем собрания (Г6)")

		return
	}
	h.demoAccelerator(c, "finish voting", h.meetings.FinishVoting)
}

func (h *handlers) demoFillBallots(c *gin.Context) {
	if h.meetings == nil {
		notImplemented(c, "Демо-ускорители появятся вместе с модулем собрания (Г6)")

		return
	}
	h.demoAccelerator(c, "fill ballots", h.meetings.FillBallots)
}

// demoAccelerator runs a demo accelerator of the meeting and answers with its card.
func (h *handlers) demoAccelerator(c *gin.Context, what string,
	run func(ctx context.Context, meetingID, byUserID string) (meeting.View, error),
) {
	id, ok := IdentityFrom(c)
	if !ok {
		writeError(c, http.StatusInternalServerError, "internal", "Внутренняя ошибка, попробуйте ещё раз")

		return
	}

	view, err := run(c.Request.Context(), c.Param("id"), id.UserID)
	if err != nil {
		h.writeMeetingError(c, err, what)

		return
	}

	writeJSON(c, http.StatusOK, toMeetingJSON(view))
}

const (
	decisionsMessage  = "Отметьте решение по каждому вопросу повестки: за, против или воздержался"
	gisResultsMessage = "Укажите результаты ГИС по всем вопросам и площадь онлайн-участников в м²"
)

// writeMeetingError maps the errors of the meeting module to the contract codes.
func (h *handlers) writeMeetingError(c *gin.Context, err error, what string) {
	var dates *meeting.DatesError
	switch {
	case errors.Is(err, meeting.ErrNotFound):
		writeError(c, http.StatusNotFound, "meeting_not_found", "Собрание не найдено")
	case errors.Is(err, meeting.ErrInitiativeNotFound):
		writeError(c, http.StatusNotFound, "initiative_not_found", "Инициатива не найдена")
	case errors.Is(err, meeting.ErrBallotNotFound):
		writeError(c, http.StatusNotFound, "ballot_not_found", "Бюллетень не найден в этом собрании")
	case errors.Is(err, meeting.ErrStaffOnly):
		writeError(c, http.StatusForbidden, "staff_only",
			"Это может сделать только сотрудник управляющей компании, который ведёт собрание")
	case errors.Is(err, meeting.ErrNotMember):
		writeError(c, http.StatusForbidden, "not_member", "Собрание видят подтверждённые жители дома")
	case errors.Is(err, meeting.ErrWrongStage):
		writeError(c, http.StatusConflict, "wrong_stage", "Собрание созывается после требования жителей к УК")
	case errors.Is(err, meeting.ErrActiveMeetingExists):
		writeError(c, http.StatusConflict, "active_meeting_exists", "У инициативы уже есть собрание")
	case errors.As(err, &dates):
		message, field := datesMessage(dates.Reason)
		writeFieldError(c, http.StatusBadRequest, "invalid_dates", message, field)
	case errors.Is(err, meeting.ErrInvalidOfficers):
		writeError(c, http.StatusBadRequest, "invalid_officers",
			"Председатель и секретарь — два разных собственника из реестра дома")
	case errors.Is(err, meeting.ErrInvalidForm):
		writeFieldError(c, http.StatusBadRequest, "invalid_request",
			"Форма собрания: электронное в ГИС ЖКХ или заочное на бумаге", "form")
	case errors.Is(err, meeting.ErrAlreadyReceived):
		writeError(c, http.StatusConflict, "already_received", "Бюллетень уже отмечен")
	case errors.Is(err, meeting.ErrVotingFinished):
		writeError(c, http.StatusConflict, "voting_finished",
			"Голосование окончено: бюллетени после его окончания не учитываются")
	case errors.Is(err, meeting.ErrVotingNotFinished):
		writeError(c, http.StatusConflict, "voting_not_finished",
			"Голосование ещё идёт: решения вносятся и итог фиксируется после его окончания")
	case errors.Is(err, meeting.ErrAlreadyFinalized):
		writeError(c, http.StatusConflict, "already_finalized", "Итог уже зафиксирован, изменить его нельзя")
	case errors.Is(err, meeting.ErrBallotNotReceived):
		writeError(c, http.StatusConflict, "ballot_not_received",
			"Бюллетень не отмечен полученным до окончания голосования и не учитывается")
	case errors.Is(err, meeting.ErrInvalidDecisions):
		writeError(c, http.StatusBadRequest, "invalid_request", decisionsMessage)
	case errors.Is(err, meeting.ErrInvalidGISResults):
		writeError(c, http.StatusBadRequest, "invalid_gis_results",
			"Результаты ГИС должны содержать все вопросы, а суммы голосов — совпадать с площадью участников")
	case errors.Is(err, meeting.ErrGISResultsNotAllowed):
		writeError(c, http.StatusConflict, "gis_results_not_allowed",
			"Результаты ГИС доступны только для электронного собрания в ГИС ЖКХ")
	case errors.Is(err, meeting.ErrNotDemo):
		writeError(c, http.StatusForbidden, "not_demo", "Ускорители работают только в демо-доме")
	default:
		h.log.Error(what, "err", err)
		writeError(c, http.StatusInternalServerError, "internal", "Не удалось выполнить действие, попробуйте ещё раз")
	}
}

// datesMessage explains a rejected date and names the field of the form.
func datesMessage(reason string) (message, field string) {
	switch reason {
	case meeting.DatesNoticeInPast:
		return "Дата сообщения о собрании уже прошла", "notice_at"
	case meeting.DatesStartsBeforeNotice:
		return "Голосование начинается не раньше сообщения о собрании", "voting_starts_at"
	case meeting.DatesEndsBeforeStarts:
		return "Голосование должно закончиться позже, чем начнётся", "voting_ends_at"
	case meeting.DatesNoticePeriod:
		return "Голосование начинается не раньше чем через 10 дней после сообщения о собрании (ст. 45 ч. 4 ЖК РФ)",
			"voting_starts_at"
	default:
		return "Укажите даты сообщения и голосования", ""
	}
}

type meetingJSON struct {
	ID             string                  `json:"id"`
	InitiativeID   string                  `json:"initiative_id"`
	Title          string                  `json:"title"`
	House          meetingHouseJSON        `json:"house"`
	Attempt        int                     `json:"attempt"`
	Form           string                  `json:"form"`
	Status         string                  `json:"status"`
	NoticeAt       time.Time               `json:"notice_at"`
	VotingStartsAt time.Time               `json:"voting_starts_at"`
	VotingEndsAt   time.Time               `json:"voting_ends_at"`
	Chair          officerJSON             `json:"chair"`
	Secretary      officerJSON             `json:"secretary"`
	AgendaItems    []meetingAgendaItemJSON `json:"agenda_items"`
	Progress       meetingProgressJSON     `json:"progress"`
	IsAdmin        bool                    `json:"is_admin"`
	Outcome        *string                 `json:"outcome"`
	FinalizedAt    *time.Time              `json:"finalized_at"`
}

type meetingHouseJSON struct {
	ID      string `json:"id"`
	Address string `json:"address"`
}

type officerJSON struct {
	OwnerID    string `json:"owner_id"`
	MaskedName string `json:"masked_name"`
}

type meetingAgendaItemJSON struct {
	ID           string `json:"id"`
	Position     int    `json:"position"`
	Text         string `json:"text"`
	MajorityRule string `json:"majority_rule"`
}

type meetingProgressJSON struct {
	BallotsTotal    int    `json:"ballots_total"`
	BallotsReceived int    `json:"ballots_received"`
	ParticipantsM2  string `json:"participants_m2"`
	TotalM2         string `json:"total_m2"`
	QuorumAboveM2   string `json:"quorum_above_m2"`
}

func toMeetingJSON(v meeting.View) meetingJSON {
	j := meetingJSON{
		ID:             v.ID,
		InitiativeID:   v.InitiativeID,
		Title:          v.Title,
		House:          meetingHouseJSON{ID: v.House.ID, Address: v.House.Address},
		Attempt:        v.Attempt,
		Form:           v.Form,
		Status:         v.Status,
		NoticeAt:       v.NoticeAt,
		VotingStartsAt: v.VotingStartsAt,
		VotingEndsAt:   v.VotingEndsAt,
		Chair:          officerJSON{OwnerID: v.Chair.OwnerID, MaskedName: v.Chair.MaskedName},
		Secretary:      officerJSON{OwnerID: v.Secretary.OwnerID, MaskedName: v.Secretary.MaskedName},
		AgendaItems:    make([]meetingAgendaItemJSON, 0, len(v.Agenda)),
		Progress: meetingProgressJSON{
			BallotsTotal:    v.Progress.BallotsTotal,
			BallotsReceived: v.Progress.BallotsReceived,
			ParticipantsM2:  m2(v.Progress.ParticipantsM2),
			TotalM2:         m2(v.Progress.TotalM2),
			QuorumAboveM2:   m2(v.Progress.QuorumAboveM2),
		},
		IsAdmin:     v.IsAdmin,
		Outcome:     v.Outcome,
		FinalizedAt: v.FinalizedAt,
	}
	for _, item := range v.Agenda {
		j.AgendaItems = append(j.AgendaItems, meetingAgendaItemJSON{
			ID: item.ID, Position: item.Position, Text: item.Text, MajorityRule: item.MajorityRule,
		})
	}

	return j
}

type trackerJSON struct {
	Summary trackerSummaryJSON  `json:"summary"`
	Ballots []trackerBallotJSON `json:"ballots"`
}

type trackerSummaryJSON struct {
	BallotsTotal   int    `json:"ballots_total"`
	Received       int    `json:"received"`
	Counted        int    `json:"counted"`
	ParticipantsM2 string `json:"participants_m2"`
}

type trackerBallotJSON struct {
	ID              string `json:"id"`
	PremiseNumber   string `json:"premise_number"`
	Entrance        *int   `json:"entrance"`
	OwnerMaskedName string `json:"owner_masked_name"`
	WeightM2        string `json:"weight_m2"`
	Status          string `json:"status"`
}

func toTrackerJSON(t meeting.Tracker) trackerJSON {
	j := trackerJSON{
		Summary: trackerSummaryJSON{
			BallotsTotal: t.BallotsTotal, Received: t.Received, Counted: t.Counted,
			ParticipantsM2: m2(t.ParticipantsM2),
		},
		Ballots: make([]trackerBallotJSON, 0, len(t.Ballots)),
	}
	for _, b := range t.Ballots {
		j.Ballots = append(j.Ballots, trackerBallotJSON{
			ID: b.BallotID, PremiseNumber: b.PremiseNumber, Entrance: b.Entrance,
			OwnerMaskedName: b.OwnerMaskedName, WeightM2: m2(b.WeightM2), Status: b.Status,
		})
	}

	return j
}

type receivedBallotJSON struct {
	BallotID   string    `json:"ballot_id"`
	Status     string    `json:"status"`
	ReceivedAt time.Time `json:"received_at"`
}

type decisionJSON struct {
	AgendaItemID string `json:"agenda_item_id"`
	Choice       string `json:"choice"`
}

type ballotDecisionsJSON struct {
	BallotID  string         `json:"ballot_id"`
	Status    string         `json:"status"`
	Decisions []decisionJSON `json:"decisions"`
}

type gisResultEntryJSON struct {
	AgendaItemID string `json:"agenda_item_id" binding:"required"`
	ForM2        string `json:"for_m2" binding:"required"`
	AgainstM2    string `json:"against_m2" binding:"required"`
	AbstainM2    string `json:"abstain_m2" binding:"required"`
}

type gisResultsJSON struct {
	Entries              []gisResultEntryJSON `json:"entries" binding:"required,min=1,dive"`
	OnlineParticipantsM2 string               `json:"online_participants_m2" binding:"required"`
}

func toGISResultsJSON(results meeting.GISResults) gisResultsJSON {
	j := gisResultsJSON{
		Entries:              make([]gisResultEntryJSON, 0, len(results.Entries)),
		OnlineParticipantsM2: m2(results.OnlineParticipantsM2),
	}
	for _, entry := range results.Entries {
		j.Entries = append(j.Entries, gisResultEntryJSON{
			AgendaItemID: entry.AgendaItemID,
			ForM2:        m2(entry.ForM2),
			AgainstM2:    m2(entry.AgainstM2),
			AbstainM2:    m2(entry.AbstainM2),
		})
	}

	return j
}

// parseM2Input accepts only a non-negative decimal with at most two fractional
// digits. It intentionally rejects exponent and fraction syntax accepted by big.Rat.
func parseM2Input(value string) (*big.Rat, bool) {
	whole, fraction, hasFraction := strings.Cut(value, ".")
	if !decimalDigits(whole) || (hasFraction && (!decimalDigits(fraction) || len(fraction) > 2)) {
		return nil, false
	}
	parsed, ok := new(big.Rat).SetString(value)
	if !ok || parsed.Sign() < 0 {
		return nil, false
	}
	centi := new(big.Rat).Mul(parsed, big.NewRat(100, 1))
	if centi.Denom().Cmp(big.NewInt(1)) != 0 || !centi.Num().IsInt64() {
		return nil, false
	}

	return parsed, true
}

func decimalDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}

	return true
}

type resultJSON struct {
	ParticipantsM2 string             `json:"participants_m2"`
	TotalM2        string             `json:"total_m2"`
	QuorumReached  bool               `json:"quorum_reached"`
	AgendaResults  []agendaResultJSON `json:"agenda_results"`
}

type agendaResultJSON struct {
	AgendaItemID string `json:"agenda_item_id"`
	Position     int    `json:"position"`
	Text         string `json:"text"`
	MajorityRule string `json:"majority_rule"`
	ForM2        string `json:"for_m2"`
	AgainstM2    string `json:"against_m2"`
	AbstainM2    string `json:"abstain_m2"`
	Accepted     bool   `json:"accepted"`
}

func toResultJSON(r meeting.Result) resultJSON {
	j := resultJSON{
		ParticipantsM2: m2(r.ParticipantsM2), TotalM2: m2(r.TotalM2), QuorumReached: r.QuorumReached,
		AgendaResults: make([]agendaResultJSON, 0, len(r.Items)),
	}
	for _, item := range r.Items {
		j.AgendaResults = append(j.AgendaResults, agendaResultJSON{
			AgendaItemID: item.AgendaItemID, Position: item.Position, Text: item.Text, MajorityRule: item.MajorityRule,
			ForM2: m2(item.ForM2), AgainstM2: m2(item.AgainstM2), AbstainM2: m2(item.AbstainM2), Accepted: item.Accepted,
		})
	}

	return j
}

type finalJSON struct {
	MeetingID      string             `json:"meeting_id"`
	Outcome        string             `json:"outcome"`
	FinalizedAt    time.Time          `json:"finalized_at"`
	ParticipantsM2 string             `json:"participants_m2"`
	TotalM2        string             `json:"total_m2"`
	QuorumReached  bool               `json:"quorum_reached"`
	Results        []agendaResultJSON `json:"results"`
}
