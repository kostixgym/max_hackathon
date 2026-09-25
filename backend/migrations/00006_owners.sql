-- +goose Up
CREATE TABLE owners (
    id         uuid PRIMARY KEY DEFAULT uuidv7(),
    premise_id uuid NOT NULL REFERENCES premises (id),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX owners_premise_idx ON owners (premise_id);

-- +goose Down
DROP TABLE owners;
