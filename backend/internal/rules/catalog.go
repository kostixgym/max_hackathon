package rules

import (
	"context"
	"encoding/json"
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
	// ParamsSchema is the form of the template (JSON Schema, see params.go), UISchema
	// how the mini-app shows it. Both are JSON objects.
	ParamsSchema json.RawMessage
	UISchema     json.RawMessage
	Items        []CatalogItem
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

// DecisionType is a type of decision of the meeting with the majority it needs.
type DecisionType struct {
	ID             string
	Code           string
	Name           string
	MajorityRule   string
	LegalReference string
}

// ValidateParams checks the values of the template form. A *ParamsError names the field.
func (t CatalogTemplate) ValidateParams(params []byte) error {
	schema, err := CompileParamsSchema(t.ParamsSchema)
	if err != nil {
		return fmt.Errorf("template %s v%d: %w", t.Code, t.Version, err)
	}

	return schema.Validate(params)
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

// Templates returns the latest version of every template, without the agenda and the form.
func (c *Catalog) Templates(ctx context.Context) ([]CatalogTemplate, error) {
	rows, err := c.pool.Query(ctx, `
		SELECT DISTINCT ON (code) id::text, code, version, name, coalesce(description, '')
		FROM templates
		ORDER BY code, version DESC`)
	if err != nil {
		return nil, fmt.Errorf("list templates: %w", err)
	}
	defer rows.Close()

	result := make([]CatalogTemplate, 0)
	for rows.Next() {
		var t CatalogTemplate
		if err := rows.Scan(&t.ID, &t.Code, &t.Version, &t.Name, &t.Description); err != nil {
			return nil, fmt.Errorf("scan template: %w", err)
		}
		result = append(result, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list templates: %w", err)
	}

	return result, nil
}

// TemplateByCode returns the latest version of the template with the given code.
func (c *Catalog) TemplateByCode(ctx context.Context, code string) (CatalogTemplate, error) {
	return c.template(ctx, `WHERE code = $1 ORDER BY version DESC LIMIT 1`, code)
}

// TemplateByID returns the template version with the given id: an initiative keeps
// the version it was created with.
func (c *Catalog) TemplateByID(ctx context.Context, id string) (CatalogTemplate, error) {
	return c.template(ctx, `WHERE id = $1::uuid`, id)
}

func (c *Catalog) template(ctx context.Context, where string, arg string) (CatalogTemplate, error) {
	var t CatalogTemplate
	err := c.pool.QueryRow(ctx, `
		SELECT id::text, code, version, name, coalesce(description, ''), params_schema, ui_schema
		FROM templates `+where, arg,
	).Scan(&t.ID, &t.Code, &t.Version, &t.Name, &t.Description, &t.ParamsSchema, &t.UISchema)
	if errors.Is(err, pgx.ErrNoRows) {
		return t, fmt.Errorf("%w: %q", ErrTemplateNotFound, arg)
	}
	if err != nil {
		return t, fmt.Errorf("template: %w", err)
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
			return t, fmt.Errorf("template %s item %d: %w", t.Code, item.Position, err)
		}
		t.Items = append(t.Items, item)
	}
	if err := rows.Err(); err != nil {
		return t, fmt.Errorf("template items: %w", err)
	}

	return t, nil
}

// DecisionTypes returns the decision types with the given ids. Unknown ids are skipped.
func (c *Catalog) DecisionTypes(ctx context.Context, ids []string) ([]DecisionType, error) {
	rows, err := c.pool.Query(ctx, `
		SELECT id::text, code, name, majority_rule, coalesce(legal_reference, '')
		FROM decision_types
		WHERE id = ANY($1::uuid[])`, ids)
	if err != nil {
		return nil, fmt.Errorf("decision types: %w", err)
	}
	defer rows.Close()

	result := make([]DecisionType, 0, len(ids))
	for rows.Next() {
		var d DecisionType
		if err := rows.Scan(&d.ID, &d.Code, &d.Name, &d.MajorityRule, &d.LegalReference); err != nil {
			return nil, fmt.Errorf("scan decision type: %w", err)
		}
		if _, err := ParseRule(d.MajorityRule); err != nil {
			return nil, fmt.Errorf("decision type %s: %w", d.Code, err)
		}
		result = append(result, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("decision types: %w", err)
	}

	return result, nil
}

// SeedCatalog idempotently creates the decision types and the MVP templates.
func (c *Catalog) SeedCatalog(ctx context.Context) error {
	for _, d := range decisionTypes {
		if _, err := c.pool.Exec(ctx, `
			INSERT INTO decision_types (code, name, majority_rule, legal_reference)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (code) DO UPDATE
			SET name = EXCLUDED.name, majority_rule = EXCLUDED.majority_rule,
			    legal_reference = EXCLUDED.legal_reference, updated_at = now()`,
			d.Code, d.Name, d.Rule, d.LegalReference); err != nil {
			return fmt.Errorf("seed decision type %s: %w", d.Code, err)
		}
	}

	for _, t := range templates {
		if err := c.seedTemplate(ctx, t); err != nil {
			return fmt.Errorf("seed template %s: %w", t.Code, err)
		}
	}

	return nil
}

func (c *Catalog) seedTemplate(ctx context.Context, t templateDef) error {
	// A form the mini-app cannot show must not reach the users.
	if _, err := CompileParamsSchema([]byte(t.ParamsSchema)); err != nil {
		return err
	}
	var ui map[string]any
	if err := json.Unmarshal([]byte(t.UISchema), &ui); err != nil {
		return fmt.Errorf("ui schema: %w", err)
	}

	var templateID string
	err := c.pool.QueryRow(ctx, `
		INSERT INTO templates (code, version, name, description, params_schema, ui_schema)
		VALUES ($1, $2, $3, $4, $5::jsonb, $6::jsonb)
		ON CONFLICT (code, version) DO UPDATE
		SET name = EXCLUDED.name, description = EXCLUDED.description,
		    params_schema = EXCLUDED.params_schema, ui_schema = EXCLUDED.ui_schema
		RETURNING id::text`, t.Code, t.Version, t.Name, t.Description, t.ParamsSchema, t.UISchema,
	).Scan(&templateID)
	if err != nil {
		return fmt.Errorf("upsert template: %w", err)
	}

	for _, item := range t.Items {
		tag, err := c.pool.Exec(ctx, `
			INSERT INTO template_items (template_id, position, text, decision_type_id)
			SELECT $1::uuid, $2, $3, id FROM decision_types WHERE code = $4
			ON CONFLICT (template_id, position) DO UPDATE
			SET text = EXCLUDED.text, decision_type_id = EXCLUDED.decision_type_id`,
			templateID, item.Position, item.Text, item.DecisionCode)
		if err != nil {
			return fmt.Errorf("upsert template item %d: %w", item.Position, err)
		}
		// INSERT … SELECT writes nothing for an unknown decision type: that is a typo in the catalog.
		if tag.RowsAffected() != 1 {
			return fmt.Errorf("template item %d: unknown decision type %q", item.Position, item.DecisionCode)
		}
	}

	return nil
}
