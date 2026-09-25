-- +goose Up
CREATE TABLE ballot_decisions (
    id                  uuid PRIMARY KEY DEFAULT uuidv7(),
    ballot_id           uuid NOT NULL REFERENCES ballots (id) ON DELETE CASCADE,
    agenda_item_id      uuid NOT NULL REFERENCES agenda_items (id),
    choice              text NOT NULL CHECK (choice IN ('for', 'against', 'abstain')),
    recorded_by_user_id uuid REFERENCES users (id) ON DELETE SET NULL,
    recorded_at         timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    UNIQUE (ballot_id, agenda_item_id)
);

-- +goose Down
DROP TABLE ballot_decisions;
