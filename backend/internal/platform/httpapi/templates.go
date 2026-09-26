package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"maxhackathon/backend/internal/rules"
)

// Templates of initiatives (docs/API_DESCRIPTION.md): the mini-app lists them, then
// builds the form of the chosen one from its schema. Platform data without personal
// data: any signed-in user reads it.

// TemplateCatalog reads the templates (the rules module).
type TemplateCatalog interface {
	Templates(ctx context.Context) ([]rules.CatalogTemplate, error)
	TemplateByCode(ctx context.Context, code string) (rules.CatalogTemplate, error)
}

type templateSummaryJSON struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	Version     int    `json:"version"`
	Description string `json:"description"`
}

type templateJSON struct {
	Code         string           `json:"code"`
	Name         string           `json:"name"`
	Version      int              `json:"version"`
	Description  string           `json:"description"`
	ParamsSchema json.RawMessage  `json:"params_schema"`
	UISchema     json.RawMessage  `json:"ui_schema"`
	AgendaItems  []agendaItemJSON `json:"agenda_items"`
}

type templatesResponse struct {
	Templates []templateSummaryJSON `json:"templates"`
}

func (h *handlers) listTemplates(c *gin.Context) {
	list, err := h.templates.Templates(c.Request.Context())
	if err != nil {
		h.log.Error("list templates", "err", err)
		writeError(c, http.StatusInternalServerError, "internal", "Не удалось загрузить шаблоны, попробуйте ещё раз")

		return
	}

	resp := templatesResponse{Templates: make([]templateSummaryJSON, 0, len(list))}
	for _, t := range list {
		resp.Templates = append(resp.Templates, templateSummaryJSON{
			Code: t.Code, Name: t.Name, Version: t.Version, Description: t.Description,
		})
	}
	writeJSON(c, http.StatusOK, resp)
}

func (h *handlers) template(c *gin.Context) {
	t, err := h.templates.TemplateByCode(c.Request.Context(), c.Param("code"))
	if errors.Is(err, rules.ErrTemplateNotFound) {
		writeError(c, http.StatusNotFound, "template_not_found", "Шаблон не найден")

		return
	}
	if err != nil {
		h.log.Error("template by code", "err", err)
		writeError(c, http.StatusInternalServerError, "internal", "Не удалось загрузить шаблон, попробуйте ещё раз")

		return
	}

	resp := templateJSON{
		Code: t.Code, Name: t.Name, Version: t.Version, Description: t.Description,
		ParamsSchema: t.ParamsSchema, UISchema: t.UISchema,
		AgendaItems: make([]agendaItemJSON, 0, len(t.Items)),
	}
	for _, item := range t.Items {
		resp.AgendaItems = append(resp.AgendaItems, agendaItemJSON{
			Position: item.Position, Text: item.Text, MajorityRule: item.MajorityRule,
			LegalReference: item.LegalReference,
		})
	}
	writeJSON(c, http.StatusOK, resp)
}
