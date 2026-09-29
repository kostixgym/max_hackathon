package httpapi

// The staff part of the API (К2 of docs/plan-do-30-09.md): who the user works
// for, the houses of the organization and the demo-staff shortcut. The demands
// list of an organization lands here together with Дима's demand module (Д1).

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"maxhackathon/backend/internal/access"
	"maxhackathon/backend/internal/demand"
	"maxhackathon/backend/internal/registry"
)

// Orgs answers staff questions (the access module) and reads the houses of an
// organization (the registry module).
type Orgs interface {
	OrgsByUser(ctx context.Context, userID string) ([]access.OrgSummary, error)
	OrgByUser(ctx context.Context, userID, orgID string) (access.OrgSummary, error)
	ConfirmDemoStaff(ctx context.Context, userID, houseID string) (access.OrgSummary, error)
}

// OrgHouses lists the houses of an organization (the registry module).
type OrgHouses interface {
	HousesByOrg(ctx context.Context, orgID string) ([]registry.HouseRef, error)
}

// validOrgID keeps malformed ids out of the module queries.
func validOrgID(v string) bool { return resourceIDOk(v) }

// orgsWired reports whether the orgs dependency is wired; a missing dependency
// is a wiring mistake and answers 503, like any half-deployed build.
func (h *handlers) orgsWired(c *gin.Context) bool {
	if h.orgs == nil || h.orgHouses == nil {
		writeError(c, http.StatusServiceUnavailable, "not_ready", "Кабинет УК временно недоступен")

		return false
	}

	return true
}

func (h *handlers) myOrgs(c *gin.Context) {
	if !h.orgsWired(c) {
		return
	}

	id, ok := IdentityFrom(c)
	if !ok {
		writeError(c, http.StatusInternalServerError, "internal", "Внутренняя ошибка, попробуйте ещё раз")

		return
	}

	orgs, err := h.orgs.OrgsByUser(c.Request.Context(), id.UserID)
	if err != nil {
		h.log.Error("list orgs", "err", err)
		writeError(c, http.StatusInternalServerError, "internal", "Не удалось загрузить организации, попробуйте ещё раз")

		return
	}

	writeJSON(c, http.StatusOK, gin.H{"orgs": toOrgsJSON(orgs)})
}

func (h *handlers) orgHousesList(c *gin.Context) {
	if !h.orgsWired(c) {
		return
	}

	id, ok := IdentityFrom(c)
	if !ok {
		writeError(c, http.StatusInternalServerError, "internal", "Внутренняя ошибка, попробуйте ещё раз")

		return
	}

	orgID := c.Param("orgID")
	if !validOrgID(orgID) {
		writeError(c, http.StatusNotFound, "org_not_found", "Организация не найдена")

		return
	}

	if _, err := h.orgs.OrgByUser(c.Request.Context(), id.UserID, orgID); err != nil {
		if errors.Is(err, access.ErrForbidden) {
			writeError(c, http.StatusForbidden, "not_staff", "Это кабинет другой организации")

			return
		}
		h.log.Error("check org", "err", err)
		writeError(c, http.StatusInternalServerError, "internal", "Не удалось загрузить организации, попробуйте ещё раз")

		return
	}

	houses, err := h.orgHouses.HousesByOrg(c.Request.Context(), orgID)
	if err != nil {
		h.log.Error("houses by org", "err", err)
		writeError(c, http.StatusInternalServerError, "internal", "Не удалось загрузить дома, попробуйте ещё раз")

		return
	}

	list := make([]orgHouseJSON, 0, len(houses))
	for _, house := range houses {
		list = append(list, orgHouseJSON{ID: house.ID, Slug: house.InviteSlug, Address: house.Address, Region: house.Region, IsDemo: house.IsDemo})
	}
	writeJSON(c, http.StatusOK, gin.H{"houses": list})
}

func (h *handlers) orgDemands(c *gin.Context) {
	if !h.orgsWired(c) {
		return
	}
	if h.demands == nil {
		writeError(c, http.StatusServiceUnavailable, "not_ready", "Требования временно недоступны")
		return
	}
	id, ok := IdentityFrom(c)
	if !ok {
		writeError(c, http.StatusInternalServerError, "internal", "Внутренняя ошибка, попробуйте ещё раз")
		return
	}
	orgID := c.Param("orgID")
	if !validOrgID(orgID) {
		writeError(c, http.StatusNotFound, "org_not_found", "Организация не найдена")
		return
	}
	if _, err := h.orgs.OrgByUser(c.Request.Context(), id.UserID, orgID); err != nil {
		if errors.Is(err, access.ErrForbidden) {
			writeError(c, http.StatusForbidden, "not_staff", "Это кабинет другой организации")
			return
		}
		if errors.Is(err, access.ErrNotFound) {
			writeError(c, http.StatusNotFound, "org_not_found", "Организация не найдена")
			return
		}
		h.log.Error("check org for demands", "err", err)
		writeError(c, http.StatusInternalServerError, "internal", "Не удалось загрузить требования")
		return
	}
	houses, err := h.orgHouses.HousesByOrg(c.Request.Context(), orgID)
	if err != nil {
		h.log.Error("houses for demands", "err", err)
		writeError(c, http.StatusInternalServerError, "internal", "Не удалось загрузить требования")
		return
	}
	houseIDs := make([]string, 0, len(houses))
	houseNames := make(map[string]string, len(houses))
	for _, house := range houses {
		houseIDs = append(houseIDs, house.ID)
		houseNames[house.ID] = house.Address
	}
	demands, err := h.demands.ListByHouses(c.Request.Context(), houseIDs)
	if err != nil {
		h.log.Error("list org demands", "err", err)
		writeError(c, http.StatusInternalServerError, "internal", "Не удалось загрузить требования")
		return
	}
	list := make([]gin.H, 0, len(demands))
	for _, item := range demands {
		visible, err := h.demands.Get(c.Request.Context(), item.ID, id.UserID)
		if errors.Is(err, demand.ErrForbidden) {
			continue
		}
		if err != nil {
			h.log.Error("read org demand", "err", err)
			writeError(c, http.StatusInternalServerError, "internal", "Не удалось загрузить требования")
			return
		}
		row := toDemandJSON(visible)
		initiative, err := h.initiativeReader.Details(c.Request.Context(), visible.InitiativeID)
		if err != nil {
			h.log.Error("read org demand initiative", "err", err)
			writeError(c, http.StatusInternalServerError, "internal", "Не удалось загрузить требования")
			return
		}
		row["initiative_title"] = initiative.Title
		row["house"] = gin.H{"id": visible.HouseID, "address": houseNames[visible.HouseID]}
		row["house_id"] = visible.HouseID
		row["house_address"] = houseNames[visible.HouseID]
		list = append(list, row)
	}
	writeJSON(c, http.StatusOK, gin.H{"demands": list})
}

func (h *handlers) demoStaff(c *gin.Context) {
	if !h.orgsWired(c) {
		return
	}

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
		h.log.Error("house by slug for demo staff", "err", err)
		writeError(c, http.StatusInternalServerError, "internal", "Не удалось стать сотрудником УК, попробуйте ещё раз")

		return
	}

	org, err := h.orgs.ConfirmDemoStaff(c.Request.Context(), id.UserID, house.ID)
	switch {
	case errors.Is(err, access.ErrForbidden):
		writeError(c, http.StatusForbidden, "not_staff", "Ваш MAX ID не добавлен в список сотрудников УК")
	case errors.Is(err, access.ErrNotDemo):
		writeError(c, http.StatusForbidden, "not_demo", "Быстрый вход возможен только в демо-доме")
	case errors.Is(err, access.ErrNotFound):
		writeError(c, http.StatusNotFound, "house_not_found", "Дом не найден. Проверьте ссылку от управляющей компании")
	case errors.Is(err, access.ErrNoOrg):
		writeError(c, http.StatusConflict, "no_org", "У демо-дома нет управляющей организации")
	case err != nil:
		h.log.Error("demo staff", "err", err)
		writeError(c, http.StatusInternalServerError, "internal", "Не удалось стать сотрудником УК, попробуйте ещё раз")
	default:
		// Контракт: {org: {id, name, type}, role}. role не дублируется внутри org.
		writeJSON(c, http.StatusOK, gin.H{
			"org":  gin.H{"id": org.ID, "name": org.Name, "type": org.Type},
			"role": org.Role,
		})
	}
}

type orgJSON struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
	Role string `json:"role,omitempty"`
}

type orgHouseJSON struct {
	ID      string `json:"id"`
	Slug    string `json:"slug"`
	Address string `json:"address"`
	Region  string `json:"region"`
	IsDemo  bool   `json:"is_demo"`
}

func toOrgJSON(o access.OrgSummary) orgJSON {
	return orgJSON{ID: o.ID, Name: o.Name, Type: o.Type, Role: o.Role}
}

func toOrgsJSON(orgs []access.OrgSummary) []orgJSON {
	result := make([]orgJSON, 0, len(orgs))
	for _, o := range orgs {
		result = append(result, toOrgJSON(o))
	}

	return result
}
