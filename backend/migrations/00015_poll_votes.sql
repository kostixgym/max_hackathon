-- +goose Up
CREATE TABLE poll_votes (
    id                 uuid PRIMARY KEY DEFAULT uuidv7(),
    initiative_id      uuid NOT NULL REFERENCES initiatives (id) ON DELETE CASCADE,
    owner_id           uuid NOT NULL REFERENCES owners (id),
    user_id            uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    choice             text NOT NULL CHECK (choice IN ('for', 'against')),
    weight_num         bigint NOT NULL CHECK (weight_num > 0),
    weight_den         bigint NOT NULL CHECK (weight_den > 0),
    official_channel   text CHECK (official_channel IN ('gosuslugi', 'paper')),
    willing_to_help    boolean NOT NULL DEFAULT false,
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now(),
    UNIQUE (initiative_id, owner_id),
    CHECK (choice = 'for' OR (official_channel IS NULL AND NOT willing_to_help))
);

CREATE INDEX poll_votes_user_idx ON poll_votes (user_id);

-- +goose Down
DROP TABLE poll_votes;
