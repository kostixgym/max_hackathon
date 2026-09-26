-- +goose Up
CREATE TABLE users (
    id          uuid PRIMARY KEY DEFAULT uuidv7(),
    max_user_id bigint NOT NULL UNIQUE,
    consent_at  timestamptz,
    deleted_at  timestamptz,
    created_at  timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE users;
