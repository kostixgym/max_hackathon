package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"maxhackathon/backend/internal/access"
)

type claimOwnerService interface {
	ClaimCandidates(context.Context, string, string) ([]access.OwnerSummary, error)
	ClaimOwner(context.Context, string, string, string) error
}

type claimManager interface {
	PendingOwnerClaims(context.Context, []string) ([]access.OwnerClaimRequest, error)
	DecideOwnerClaim(context.Context, string, string, bool, string) error
}

func (h *handlers) claimCandidates(c *gin.Context) {
	service, ok := h.access.(claimOwnerService)
	if !ok {
		writeError(c, http.StatusServiceUnavailable, "not_ready", "Подтверждение через УК временно недоступно")
		return
	}
	id, exists := IdentityFrom(c)
	if !exists {
		writeError(c, 500, "internal", "Внутренняя ошибка")
		return
	}
	owners, err := service.ClaimCandidates(c.Request.Context(), id.UserID, c.Param("id"))
	switch {
	case err == nil:
		list := make([]ownerJSON, 0, len(owners))
		for _, owner := range owners {
			list = append(list, toOwnerJSON(owner, false))
		}
		writeJSON(c, http.StatusOK, gin.H{"owners": list})
	case errors.Is(err, access.ErrNotFound):
		writeError(c, 404, "membership_not_found", "Заявка не найдена")
	case err != nil:
		h.log.Error("owner claim candidates", "err", err)
		writeError(c, 500, "internal", "Не удалось загрузить список собственников")
	}
}

func (h *handlers) claimOwner(c *gin.Context) {
	service, ok := h.access.(claimOwnerService)
	if !ok {
		writeError(c, http.StatusServiceUnavailable, "not_ready", "Подтверждение через УК временно недоступно")
		return
	}
	id, exists := IdentityFrom(c)
	if !exists {
		writeError(c, 500, "internal", "Внутренняя ошибка")
		return
	}
	var body struct {
		OwnerID string `json:"owner_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		writeError(c, 400, "invalid_request", "Выберите запись собственника из реестра")
		return
	}
	err := service.ClaimOwner(c.Request.Context(), id.UserID, c.Param("id"), body.OwnerID)
	switch {
	case err == nil:
		writeJSON(c, http.StatusOK, gin.H{"status": "pending", "role": "owner"})
	case errors.Is(err, access.ErrNotFound):
		writeError(c, 404, "claim_not_found", "Заявка или запись собственника не найдена")
	case err != nil:
		h.log.Error("submit owner claim", "err", err)
		writeError(c, 500, "internal", "Не удалось отправить заявку")
	}
}

func (h *handlers) orgOwnerRequests(c *gin.Context) {
	requests, ok := h.authorizedOwnerRequests(c)
	if !ok {
		return
	}
	list := make([]gin.H, 0, len(requests))
	for _, request := range requests {
		list = append(list, claimJSON(request))
	}
	writeJSON(c, http.StatusOK, gin.H{"requests": list})
}

func (h *handlers) approveOwnerRequest(c *gin.Context) { h.decideOwnerRequest(c, true) }
func (h *handlers) rejectOwnerRequest(c *gin.Context)  { h.decideOwnerRequest(c, false) }

func (h *handlers) decideOwnerRequest(c *gin.Context, approve bool) {
	requests, ok := h.authorizedOwnerRequests(c)
	if !ok {
		return
	}
	membershipID := c.Param("membershipID")
	var found bool
	for _, request := range requests {
		if request.MembershipID == membershipID {
			found = true
			break
		}
	}
	if !found {
		writeError(c, 404, "request_not_found", "Заявка не найдена среди домов вашей организации")
		return
	}
	id, _ := IdentityFrom(c)
	reason := ""
	if !approve {
		var body struct {
			Reason string `json:"reason" binding:"required,max=500"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			writeError(c, 400, "invalid_request", "Укажите причину отклонения (до 500 символов)")
			return
		}
		reason = strings.TrimSpace(body.Reason)
		if reason == "" {
			writeError(c, 400, "invalid_request", "Укажите причину отклонения")
			return
		}
	}
	manager, ok := h.access.(claimManager)
	if !ok {
		writeError(c, 503, "not_ready", "Обработка заявок временно недоступна")
		return
	}
	if err := manager.DecideOwnerClaim(c.Request.Context(), membershipID, id.UserID, approve, reason); err != nil {
		if errors.Is(err, access.ErrNotFound) {
			writeError(c, 409, "request_processed", "Заявка уже обработана")
			return
		}
		if errors.Is(err, access.ErrOwnerTaken) {
			writeError(c, 409, "owner_already_verified", "Эта запись собственника уже подтверждена за другим аккаунтом")
			return
		}
		h.log.Error("decide owner request", "err", err)
		writeError(c, 500, "internal", "Не удалось обработать заявку")
		return
	}
	status := "rejected"
	if approve {
		status = "verified"
	}
	writeJSON(c, http.StatusOK, gin.H{"membership_id": membershipID, "status": status})
}

func (h *handlers) authorizedOwnerRequests(c *gin.Context) ([]access.OwnerClaimRequest, bool) {
	if !h.orgsWired(c) {
		return nil, false
	}
	id, ok := IdentityFrom(c)
	if !ok {
		writeError(c, 500, "internal", "Внутренняя ошибка")
		return nil, false
	}
	orgID := c.Param("orgID")
	if !validOrgID(orgID) {
		writeError(c, 404, "org_not_found", "Организация не найдена")
		return nil, false
	}
	if _, err := h.orgs.OrgByUser(c.Request.Context(), id.UserID, orgID); err != nil {
		if errors.Is(err, access.ErrForbidden) {
			writeError(c, 403, "not_staff", "Это кабинет другой организации")
		} else {
			writeError(c, 404, "org_not_found", "Организация не найдена")
		}
		return nil, false
	}
	houses, err := h.orgHouses.HousesByOrg(c.Request.Context(), orgID)
	if err != nil {
		h.log.Error("owner requests houses", "err", err)
		writeError(c, 500, "internal", "Не удалось загрузить заявки")
		return nil, false
	}
	houseIDs := make([]string, 0, len(houses))
	for _, house := range houses {
		houseIDs = append(houseIDs, house.ID)
	}
	manager, ok := h.access.(claimManager)
	if !ok {
		writeError(c, 503, "not_ready", "Обработка заявок временно недоступна")
		return nil, false
	}
	requests, err := manager.PendingOwnerClaims(c.Request.Context(), houseIDs)
	if err != nil {
		h.log.Error("list owner requests", "err", err)
		writeError(c, 500, "internal", "Не удалось загрузить заявки")
		return nil, false
	}
	return requests, true
}

func claimJSON(request access.OwnerClaimRequest) gin.H {
	return gin.H{"membership_id": request.MembershipID, "owner": toOwnerJSON(request.Owner, true)}
}
