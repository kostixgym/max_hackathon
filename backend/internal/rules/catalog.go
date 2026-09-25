package rules

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// The rules module owns decision types and initiative templates (docs/04, module 3).
// The platform is the only writer: the catalog is seeded idempotently on start.

// CatalogTemplate is a template with its agenda items, as other modules get it.
type CatalogTemplate struct {
	ID          string
	Code        string
	Version     int
	Name        string
	Description string
	Items       []CatalogItem
}

// CatalogItem is one agenda item of a template.
type CatalogItem struct {
	Position       int
	Text           string
	DecisionCode   string
	DecisionTypeID string
	MajorityRule   string
	LegalReference string
}

// Catalog is the read-and-seed API of decision types and templates.
type Catalog struct {
	pool *pgxpool.Pool
}

// NewCatalog creates a rules catalog store.
func NewCatalog(pool *pgxpool.Pool) *Catalog {
	return &Catalog{pool: pool}
}

// ErrTemplateNotFound means that no template with the code exists.
var ErrTemplateNotFound = errors.New("template not found")

// TemplateByCode returns the latest version of the template with the given code.
func (c *Catalog) TemplateByCode(ctx context.Context, code string) (CatalogTemplate, error) {
	var t CatalogTemplate
	var description *string
	err := c.pool.QueryRow(ctx, `
		SELECT id::text, code, version, name, description
		FROM templates
		WHERE code = $1
		ORDER BY version DESC
		LIMIT 1`, code,
	).Scan(&t.ID, &t.Code, &t.Version, &t.Name, &description)
	if errors.Is(err, pgx.ErrNoRows) {
		return t, fmt.Errorf("%w: %q", ErrTemplateNotFound, code)
	}
	if err != nil {
		return t, fmt.Errorf("template by code: %w", err)
	}
	if description != nil {
		t.Description = *description
	}

	rows, err := c.pool.Query(ctx, `
		SELECT i.position, i.text, d.code, d.id::text, d.majority_rule, coalesce(d.legal_reference, '')
		FROM template_items i
		JOIN decision_types d ON d.id = i.decision_type_id
		WHERE i.template_id = $1::uuid
		ORDER BY i.position`, t.ID)
	if err != nil {
		return t, fmt.Errorf("template items: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var item CatalogItem
		if err := rows.Scan(&item.Position, &item.Text, &item.DecisionCode, &item.DecisionTypeID,
			&item.MajorityRule, &item.LegalReference); err != nil {
			return t, fmt.Errorf("scan template item: %w", err)
		}
		if _, err := ParseRule(item.MajorityRule); err != nil {
			return t, fmt.Errorf("template %s item %d: %w", code, item.Position, err)
		}
		t.Items = append(t.Items, item)
	}
	if err := rows.Err(); err != nil {
		return t, fmt.Errorf("template items: %w", err)
	}

	return t, nil
}

// seedDecisionType upserts a decision type by code.
func (c *Catalog) seedDecisionType(ctx context.Context, code, name, rule, ref string) error {
	_, err := c.pool.Exec(ctx, `
		INSERT INTO decision_types (code, name, majority_rule, legal_reference)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (code) DO UPDATE
		SET name = EXCLUDED.name, majority_rule = EXCLUDED.majority_rule,
		    legal_reference = EXCLUDED.legal_reference, updated_at = now()`,
		code, name, rule, ref)

	return err
}

// SeedCatalog idempotently creates the decision types and the MVP templates.
func (c *Catalog) SeedCatalog(ctx context.Context) error {
	// Пользование общим имуществом (п. 3 ч. 2 ст. 44 ЖК) решается большинством не менее
	// 2/3 голосов от общего числа голосов собственников (ч. 1 ст. 46 ЖК). Видеонаблюдение
	// в подъезде попадает сюда (позиция ВС РФ, docs/00). Часть 2 статьи 46 — о другом:
	// собрание не решает вопросы вне повестки.
	if err := c.seedDecisionType(ctx,
		"common_property_use",
		"Пользование общим имуществом (в том числе видеонаблюдение)",
		string(TwoThirdsOfAll),
		"ЖК РФ, ч. 1 ст. 46, п. 3 ч. 2 ст. 44",
	); err != nil {
		return fmt.Errorf("seed decision type: %w", err)
	}
	// Ст. 46 ч. 1 ЖК: общее правило для остальных решений собрания.
	if err := c.seedDecisionType(ctx,
		"routine",
		"Обычные решения общего собрания",
		string(MajorityOfParticipants),
		"ЖК РФ, ст. 46 ч. 1",
	); err != nil {
		return fmt.Errorf("seed decision type: %w", err)
	}

	// Шаблон «Видеонаблюдение» (docs/02, шаг 1): один вопрос повестки.
	if err := c.seedTemplate(ctx, "cctv", 1,
		"Видеонаблюдение",
		"Камеры в подъездах: где ставим, кто хранит записи, как оплачиваем.",
		[]CatalogItem{{
			Position:     1,
			Text:         "Установить видеонаблюдение в подъездах дома (монтаж, хранение записей и оплата — по проекту, приложенному к материалам собрания)",
			DecisionCode: "common_property_use",
		}},
	); err != nil {
		return fmt.Errorf("seed template cctv: %w", err)
	}

	return nil
}

func (c *Catalog) seedTemplate(ctx context.Context, code string, version int, name, description string, items []CatalogItem) error {
	var templateID string
	err := c.pool.QueryRow(ctx, `
		INSERT INTO templates (code, version, name, description)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (code, version) DO UPDATE
		SET name = EXCLUDED.name, description = EXCLUDED.description
		RETURNING id::text`, code, version, name, description,
	).Scan(&templateID)
	if err != nil {
		return fmt.Errorf("upsert template: %w", err)
	}

	for _, item := range items {
		if _, err := c.pool.Exec(ctx, `
			INSERT INTO template_items (template_id, position, text, decision_type_id)
			SELECT $1::uuid, $2, $3, id FROM decision_types WHERE code = $4
			ON CONFLICT (template_id, position) DO UPDATE
			SET text = EXCLUDED.text, decision_type_id = EXCLUDED.decision_type_id`,
			templateID, item.Position, item.Text, item.DecisionCode); err != nil {
			return fmt.Errorf("upsert template item %d: %w", item.Position, err)
		}
	}

	return nil
}
