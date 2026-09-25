-- +goose Up
CREATE TABLE template_items (
    id               uuid PRIMARY KEY DEFAULT uuidv7(),
    template_id      uuid NOT NULL REFERENCES templates (id) ON DELETE CASCADE,
    position         integer NOT NULL CHECK (position > 0),
    text             text NOT NULL,
    decision_type_id uuid NOT NULL REFERENCES decision_types (id),
    created_at       timestamptz NOT NULL DEFAULT now(),
    UNIQUE (template_id, position)
);

-- +goose Down
DROP TABLE template_items;
