-- +goose Up
CREATE TABLE decision_types (
    id              uuid PRIMARY KEY DEFAULT uuidv7(),
    code            text NOT NULL UNIQUE,
    name            text NOT NULL,
    majority_rule   text NOT NULL CHECK (majority_rule IN (
        'majority_of_participants',
        'more_than_half_of_all',
        'two_thirds_of_all'
    )),
    legal_reference text,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE decision_types;
