package httpapi

// Meeting endpoints (docs/plan-do-30-09.md, Г2–Г6; the contract is the «Собрание»
// section of docs/API_DESCRIPTION.md). Who may act is decided inside the module by
// access.ManagesAsStaff: in the demo house a tester runs only the meetings of their
// own initiatives (решение 79). The protocol PDF is Дима's (documents.go).

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"maxhackathon/backend/internal/meeting"
)

// Meetings is the meeting module (nil keeps the routes at 501).
type Meetings interface {
	Create(ctx context.Context, in meeting.CreateInput) (meeting.View, error)
	Get(ctx context.Context, meetingID, viewerID string) (meeting.View, error)
	Tracker(ctx context.Context, meetingID, viewerID string) (meeting.Tracker, error)
	ReceiveBallot(ctx context.Context, meetingID, ballotID, byUserID string) (meeting.ReceivedBallot, error)
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
	notImplemented(c, "Внесение решений появится вместе с модулем собрания (Г4)")
}

func (h *handlers) meetingResultPreview(c *gin.Context) {
	notImplemented(c, "Предпросмотр итога появится вместе с модулем собрания (Г5)")
}

func (h *handlers) finalizeMeeting(c *gin.Context) {
	notImplemented(c, "Фиксация итога появится вместе с модулем собрания (Г5)")
}

func (h *handlers) demoFinishVoting(c *gin.Context) {
	notImplemented(c, "Демо-ускорители появятся вместе с модулем собрания (Г6)")
}

func (h *handlers) demoFillBallots(c *gin.Context) {
	notImplemented(c, "Демо-ускорители появятся вместе с модулем собрания (Г6)")
}

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
