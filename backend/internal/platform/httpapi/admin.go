package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"maxhackathon/backend/internal/access"
	"maxhackathon/backend/internal/registry"
)

type AdminAccess interface {
	IsSystemAdmin(context.Context, string) (bool, error)
	SearchKnownUsers(context.Context, string) ([]access.ManagedUser, error)
	AllOrganizations(context.Context) ([]registry.Org, error)
	SetOrgStaff(context.Context, int64, string, string, bool) error
}

func (h *handlers) adminOnly(c *gin.Context) (Identity, bool) {
	id, ok := IdentityFrom(c)
	if !ok {
		writeError(c, http.StatusInternalServerError, "internal", "Внутренняя ошибка")
		return Identity{}, false
	}
	if h.admin == nil {
		writeError(c, http.StatusServiceUnavailable, "not_ready", "Управление доступом временно недоступно")
		return Identity{}, false
	}
	allowed, err := h.admin.IsSystemAdmin(c.Request.Context(), id.UserID)
	if err != nil {
		h.log.Error("check admin access", "err", err)
		writeError(c, http.StatusInternalServerError, "internal", "Не удалось проверить права")
		return Identity{}, false
	}
	if !allowed {
		writeError(c, http.StatusForbidden, "admin_required", "Доступно только системному администратору")
		return Identity{}, false
	}
	return id, true
}

func (h *handlers) adminOrganizations(c *gin.Context) {
	if _, ok := h.adminOnly(c); !ok {
		return
	}
	orgs, err := h.admin.AllOrganizations(c.Request.Context())
	if err != nil {
		h.log.Error("admin list organizations", "err", err)
		writeError(c, 500, "internal", "Не удалось загрузить организации")
		return
	}
	list := make([]orgJSON, 0, len(orgs))
	for _, org := range orgs {
		list = append(list, orgJSON{ID: org.ID, Name: org.Name, Type: org.Type})
	}
	writeJSON(c, http.StatusOK, gin.H{"orgs": list})
}

func (h *handlers) adminUsers(c *gin.Context) {
	if _, ok := h.adminOnly(c); !ok {
		return
	}
	query := strings.TrimSpace(c.Query("q"))
	if len(query) < 2 || strings.Trim(query, "0123456789") != "" {
		writeError(c, http.StatusBadRequest, "query_too_short", "Введите не менее двух цифр MAX ID")
		return
	}
	users, err := h.admin.SearchKnownUsers(c.Request.Context(), query)
	if err != nil {
		h.log.Error("admin search users", "err", err)
		writeError(c, 500, "internal", "Не удалось найти пользователей")
		return
	}
	list := make([]gin.H, 0, len(users))
	for _, user := range users {
		list = append(list, gin.H{"max_user_id": strconv.FormatInt(user.MaxUserID, 10)})
	}
	writeJSON(c, http.StatusOK, gin.H{"users": list})
}

func (h *handlers) adminSetOrgStaff(c *gin.Context) {
	if _, ok := h.adminOnly(c); !ok {
		return
	}
	var req struct {
		MaxUserID string `json:"max_user_id"`
		OrgID     string `json:"org_id"`
		Role      string `json:"role"`
		Action    string `json:"action"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, 400, "invalid_request", "Проверьте MAX ID, организацию, роль и действие")
		return
	}
	maxUserID, parseErr := strconv.ParseInt(req.MaxUserID, 10, 64)
	if parseErr != nil || maxUserID <= 0 || !validOrgID(req.OrgID) || (req.Role != "operator" && req.Role != "admin") || (req.Action != "grant" && req.Action != "revoke") {
		writeError(c, 400, "invalid_request", "Проверьте MAX ID, организацию, роль и действие")
		return
	}
	err := h.admin.SetOrgStaff(c.Request.Context(), maxUserID, req.OrgID, req.Role, req.Action == "grant")
	switch {
	case errors.Is(err, access.ErrNotFound):
		writeError(c, 404, "user_or_org_not_found", "Пользователь или организация не найдены")
	case errors.Is(err, access.ErrInvalidRole):
		writeError(c, 400, "invalid_role", "Неизвестная роль")
	case err != nil:
		h.log.Error("admin change org staff", "err", err)
		writeError(c, 500, "internal", "Не удалось изменить доступ")
	default:
		writeJSON(c, http.StatusOK, gin.H{"ok": true})
	}
}
