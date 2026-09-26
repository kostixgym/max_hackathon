package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"maxhackathon/backend/internal/access"
	"maxhackathon/backend/internal/registry"
)

// Profiles provides access-controlled read models for the mini-app profile.
type Profiles interface {
	MembershipsByUser(ctx context.Context, userID string) ([]access.MembershipSummary, error)
	PremiseOwners(ctx context.Context, userID, premiseID string) ([]access.OwnerSummary, error)
	HouseOfficerCandidates(ctx context.Context, userID, houseID string) ([]access.OwnerSummary, error)
}

type membershipJSON struct {
	ID      string                `json:"id"`
	Role    string                `json:"role"`
	Status  string                `json:"status"`
	Method  *string               `json:"method"`
	House   membershipHouseJSON   `json:"house"`
	Premise membershipPremiseJSON `json:"premise"`
	Owner   *ownerJSON            `json:"owner"`
}

type membershipHouseJSON struct {
	ID      string `json:"id"`
	Slug    string `json:"slug"`
	Address string `json:"address"`
	Region  string `json:"region"`
	IsDemo  bool   `json:"is_demo"`
}

type membershipPremiseJSON struct {
	ID            string  `json:"id"`
	Number        string  `json:"number"`
	Kind          string  `json:"kind"`
	Entrance      *int    `json:"entrance"`
	Floor         *int    `json:"floor"`
	DisplayAreaM2 *string `json:"display_area_m2"`
}

type ownerJSON struct {
	ID         string            `json:"id"`
	MaskedName string            `json:"masked_name"`
	Kind       string            `json:"kind"`
	Share      shareJSON         `json:"share"`
	WeightM2   string            `json:"weight_m2"`
	Premise    *ownerPremiseJSON `json:"premise,omitempty"`
}

type shareJSON struct {
	Numerator   int64 `json:"numerator"`
	Denominator int64 `json:"denominator"`
}

type ownerPremiseJSON struct {
	ID     string `json:"id"`
	Number string `json:"number"`
}

type ownersResponse struct {
	Owners []ownerJSON `json:"owners"`
}

func (h *handlers) premiseOwners(c *gin.Context) {
	id, ok := IdentityFrom(c)
	if !ok {
		h.log.Error("premise owners: no identity in context")
		writeError(c, http.StatusInternalServerError, "internal", "Внутренняя ошибка, попробуйте ещё раз")

		return
	}

	owners, err := h.profiles.PremiseOwners(c.Request.Context(), id.UserID, c.Param("premiseID"))
	if err != nil {
		h.writeOwnerDirectoryError(c, err, "premise")

		return
	}

	resp := ownersResponse{Owners: make([]ownerJSON, 0, len(owners))}
	for _, owner := range owners {
		resp.Owners = append(resp.Owners, toOwnerJSON(owner, false))
	}
	writeJSON(c, http.StatusOK, resp)
}

func (h *handlers) meetingOfficerCandidates(c *gin.Context) {
	id, ok := IdentityFrom(c)
	if !ok {
		h.log.Error("meeting officer candidates: no identity in context")
		writeError(c, http.StatusInternalServerError, "internal", "Внутренняя ошибка, попробуйте ещё раз")

		return
	}

	owners, err := h.profiles.HouseOfficerCandidates(c.Request.Context(), id.UserID, c.Param("house"))
	if err != nil {
		h.writeOwnerDirectoryError(c, err, "house")

		return
	}

	resp := ownersResponse{Owners: make([]ownerJSON, 0, len(owners))}
	for _, owner := range owners {
		resp.Owners = append(resp.Owners, toOwnerJSON(owner, true))
	}
	writeJSON(c, http.StatusOK, resp)
}

func (h *handlers) writeOwnerDirectoryError(c *gin.Context, err error, resource string) {
	switch {
	case errors.Is(err, access.ErrNotFound) && resource == "premise":
		writeError(c, http.StatusNotFound, "premise_not_found", "Помещение не найдено")
	case errors.Is(err, access.ErrNotFound):
		writeError(c, http.StatusNotFound, "house_not_found", "Дом не найден")
	case errors.Is(err, access.ErrForbidden):
		writeError(c, http.StatusForbidden, "forbidden", "Недостаточно прав для просмотра собственников")
	default:
		h.log.Error("load owner directory", "resource", resource, "err", err)
		writeError(c, http.StatusInternalServerError, "internal", "Не удалось загрузить собственников, попробуйте ещё раз")
	}
}

func toMembershipJSON(m access.MembershipSummary) membershipJSON {
	house := m.Premise.House
	result := membershipJSON{
		ID: m.ID, Role: m.Role, Status: m.Status, Method: m.Method,
		House: membershipHouseJSON{
			ID: house.ID, Slug: house.InviteSlug, Address: house.Address,
			Region: house.Region, IsDemo: house.IsDemo,
		},
		Premise: membershipPremiseJSON{
			ID: m.Premise.ID, Number: m.Premise.Number, Kind: m.Premise.Kind,
			Entrance: m.Premise.Entrance, Floor: m.Premise.Floor,
		},
	}
	if m.Premise.DisplayAreaCenti != nil {
		area := m2(registry.CentiToM2(*m.Premise.DisplayAreaCenti))
		result.Premise.DisplayAreaM2 = &area
	}
	if m.Owner != nil {
		owner := toOwnerJSON(*m.Owner, false)
		result.Owner = &owner
	}

	return result
}

func toOwnerJSON(owner access.OwnerSummary, includePremise bool) ownerJSON {
	result := ownerJSON{
		ID: owner.ID, MaskedName: owner.MaskedName, Kind: owner.Kind,
		Share:    shareJSON{Numerator: owner.ShareNum, Denominator: owner.ShareDen},
		WeightM2: m2(registry.Weight{Num: owner.WeightNum, Den: owner.WeightDen}.M2()),
	}
	if includePremise {
		result.Premise = &ownerPremiseJSON{ID: owner.PremiseID, Number: owner.PremiseNumber}
	}

	return result
}
