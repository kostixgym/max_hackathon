-- +goose Up
CREATE TABLE agenda_items (
    id               uuid PRIMARY KEY DEFAULT uuidv7(),
    initiative_id    uuid NOT NULL REFERENCES initiatives (id) ON DELETE CASCADE,
    position         integer NOT NULL CHECK (position > 0),
    text             text NOT NULL,
    decision_type_id uuid NOT NULL REFERENCES decision_types (id),
    created_at       timestamptz NOT NULL DEFAULT now(),
    UNIQUE (initiative_id, position)
);

-- +goose Down
DROP TABLE agenda_items;
