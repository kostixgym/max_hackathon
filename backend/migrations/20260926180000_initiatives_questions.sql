-- +goose Up
-- Questions of neighbours to the initiator (docs/04, решения 30 и 78). The asker's
-- name is never shown to the initiator; a deleted account takes its questions along
-- (решение 28). A question is answered once.
CREATE TABLE initiative_questions (
    id                  uuid PRIMARY KEY DEFAULT uuidv7(),
    initiative_id       uuid NOT NULL REFERENCES initiatives (id) ON DELETE CASCADE,
    asked_by_user_id    uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    text                text NOT NULL CHECK (char_length(text) BETWEEN 1 AND 1000),
    answer              text CHECK (char_length(answer) BETWEEN 1 AND 2000),
    answered_by_user_id uuid REFERENCES users (id) ON DELETE SET NULL,
    answered_at         timestamptz,
    created_at          timestamptz NOT NULL DEFAULT now(),
    CHECK ((answer IS NULL) = (answered_at IS NULL))
);

-- The daily limit counts the questions of one user to one initiative.
CREATE INDEX initiative_questions_asker_idx ON initiative_questions (initiative_id, asked_by_user_id, created_at);

-- +goose Down
DROP TABLE initiative_questions;
