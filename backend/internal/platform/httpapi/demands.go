package httpapi

// Demand endpoints (Д1–Д3, docs/plan-do-30-09.md). The routes, error codes and
// the Deps field are final: Дима заменяет заглушки методами модуля demand.
// Доступ решает модуль: инициатор и сотрудник УК, который ведёт инициативу.

import (
	"context"
	"errors"
	"math/big"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"maxhackathon/backend/internal/demand"
)

// Demands is the demand module (Дима adds the methods in Д1, h.demands is wired
// in main.go).
type Demands interface {
	Create(ctx context.Context, initiativeID, byUserID, channel string) (demand.Demand, error)
	Get(ctx context.Context, demandID, viewerID string) (demand.Demand, error)
	MarkDelivered(ctx context.Context, demandID, byUserID string, deliveredAt time.Time) (demand.Demand, error)
	PDF(ctx context.Context, demandID, viewerID string) ([]byte, error)
}

func (h *handlers) createDemand(c *gin.Context) {
	id, ok := IdentityFrom(c)
	if !ok {
		writeError(c, http.StatusInternalServerError, "internal", "Внутренняя ошибка, попробуйте ещё раз")

		return
	}

	var body struct {
		Channel string `json:"channel" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "Укажите форму проведения собрания")

		return
	}

	d, err := h.demands.Create(c.Request.Context(), c.Param("id"), id.UserID, body.Channel)
	mapDemandErr(c, h.log, err, "Не удалось создать требование, попробуйте ещё раз")
	if err == nil {
		writeJSON(c, http.StatusCreated, toDemandJSON(d))
	}
}

func (h *handlers) getDemand(c *gin.Context) {
	id, ok := IdentityFrom(c)
	if !ok {
		writeError(c, http.StatusInternalServerError, "internal", "Внутренняя ошибка, попробуйте ещё раз")

		return
	}

	d, err := h.demands.Get(c.Request.Context(), c.Param("id"), id.UserID)
	mapDemandErr(c, h.log, err, "Не удалось загрузить требование, попробуйте ещё раз")
	if err == nil {
		writeJSON(c, http.StatusOK, toDemandJSON(d))
	}
}

func (h *handlers) markDemandDelivered(c *gin.Context) {
	id, ok := IdentityFrom(c)
	if !ok {
		writeError(c, http.StatusInternalServerError, "internal", "Внутренняя ошибка, попробуйте ещё раз")

		return
	}

	var body struct {
		DeliveredAt time.Time `json:"delivered_at"`
	}
	_ = c.ShouldBindJSON(&body) // тело необязательно: по умолчанию — сейчас

	d, err := h.demands.MarkDelivered(c.Request.Context(), c.Param("id"), id.UserID, body.DeliveredAt)
	mapDemandErr(c, h.log, err, "Не удалось отметить передачу, попробуйте ещё раз")
	if err == nil {
		writeJSON(c, http.StatusOK, toDemandJSON(d))
	}
}

func (h *handlers) demandPDF(c *gin.Context) {
	id, ok := IdentityFrom(c)
	if !ok {
		writeError(c, http.StatusInternalServerError, "internal", "Внутренняя ошибка, попробуйте ещё раз")

		return
	}

	pdf, err := h.demands.PDF(c.Request.Context(), c.Param("id"), id.UserID)
	if err != nil {
		mapDemandErr(c, h.log, err, "Не удалось собрать PDF, попробуйте ещё раз")

		return
	}

	c.Header("Content-Disposition", `inline; filename="demand.pdf"`)
	c.Data(http.StatusOK, "application/pdf", pdf)
}

// mapDemandErr translates the domain errors of the demand flow into API answers.
func mapDemandErr(c *gin.Context, logger interface {
	Error(msg string, args ...any)
}, err error, fallback string) {
	switch {
	case err == nil:
		return
	case errors.Is(err, demand.ErrNotFound):
		writeError(c, http.StatusNotFound, "demand_not_found", "Требование не найдено")
	case errors.Is(err, demand.ErrNotInitiator):
		writeError(c, http.StatusForbidden, "not_initiator", "Требование создаёт и ведёт инициатор")
	case errors.Is(err, demand.ErrForbidden):
		writeError(c, http.StatusForbidden, "forbidden", "Требование видят инициатор и сотрудники УК")
	case errors.Is(err, demand.ErrExists):
		writeError(c, http.StatusConflict, "demand_exists", "Требование уже создано")
	case errors.Is(err, demand.ErrSupportNotReached):
		writeError(c, http.StatusConflict, "support_not_reached", "Поддержки пока не хватает: нужно не меньше 10% площади дома")
	case errors.Is(err, demand.ErrWrongStage):
		writeError(c, http.StatusConflict, "wrong_stage", "Опрос уже закрыт")
	case errors.Is(err, demand.ErrAlreadyDelivered):
		writeError(c, http.StatusConflict, "already_delivered", "Передача в УК уже отмечена")
	default:
		logger.Error("demand", "err", err)
		writeError(c, http.StatusInternalServerError, "internal", fallback)
	}
}

// support_m2 и время отдаются в формате контракта: точные строки и RFC 3339.
func toDemandJSON(d demand.Demand) gin.H {
	support := new(big.Rat).SetFrac64(d.SupportNum, d.SupportDen) // сотые м²
	support.Quo(support, big.NewRat(100, 1))

	return gin.H{
		"id":            d.ID,
		"initiative_id": d.InitiativeID,
		"channel":       d.Channel,
		"status":        d.Status,
		"support_m2":    support.FloatString(2),
		"created_at":    d.CreatedAt,
		"delivered_at":  d.DeliveredAt,
		"uk_due_at":     d.UKDueAt,
		"overdue":       d.Overdue,
	}
}
