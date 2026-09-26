-- +goose Up
CREATE TABLE ballots (
    id                  uuid PRIMARY KEY DEFAULT uuidv7(),
    meeting_id          uuid NOT NULL REFERENCES meetings (id),
    owner_id            uuid NOT NULL REFERENCES owners (id),
    channel             text CHECK (channel IN ('online', 'paper')),
    status              text NOT NULL DEFAULT 'not_voted' CHECK (status IN (
        'not_voted', 'online_declared', 'paper_received', 'counted', 'invalid'
    )),
    weight_num          bigint NOT NULL CHECK (weight_num > 0),
    weight_den          bigint NOT NULL CHECK (weight_den > 0),
    qr_token            text NOT NULL UNIQUE,
    online_declared_at  timestamptz,
    received_at         timestamptz,
    received_by_user_id uuid REFERENCES users (id) ON DELETE SET NULL,
    counted_at          timestamptz,
    invalid_reason      text,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    UNIQUE (meeting_id, owner_id),
    CHECK ((status = 'online_declared') = (online_declared_at IS NOT NULL)),
    CHECK (status = 'invalid' OR invalid_reason IS NULL)
);

CREATE INDEX ballots_meeting_status_idx ON ballots (meeting_id, status);
CREATE INDEX ballots_owner_idx ON ballots (owner_id);

-- +goose Down
DROP TABLE ballots;
