-- +goose Up
CREATE TABLE initiatives (
    id                    uuid PRIMARY KEY DEFAULT uuidv7(),
    house_id              uuid NOT NULL REFERENCES houses (id),
    type                  text NOT NULL DEFAULT 'meeting' CHECK (type IN ('meeting')),
    template_id           uuid REFERENCES templates (id),
    title                 text NOT NULL,
    description           text,
    params                jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(params) = 'object'),
    stage                 text NOT NULL DEFAULT 'draft' CHECK (stage IN (
        'draft', 'poll', 'demand', 'meeting', 'completed', 'canceled'
    )),
    path                  text CHECK (path IN ('A', 'B')),
    registry_upload_id    uuid NOT NULL,
    poll_ends_at          timestamptz,
    initiator_user_id     uuid REFERENCES users (id) ON DELETE SET NULL,
    author_user_id        uuid REFERENCES users (id) ON DELETE SET NULL,
    canceled_by_user_id   uuid REFERENCES users (id) ON DELETE SET NULL,
    canceled_at           timestamptz,
    cancel_reason         text,
    hidden_by_user_id     uuid REFERENCES users (id) ON DELETE SET NULL,
    hidden_at             timestamptz,
    hidden_reason         text,
    created_at            timestamptz NOT NULL DEFAULT now(),
    updated_at            timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (house_id, registry_upload_id)
        REFERENCES registry_uploads (house_id, id),
    CHECK (stage <> 'poll' OR poll_ends_at IS NOT NULL),
    CHECK ((hidden_at IS NULL) = (hidden_reason IS NULL)),
    CHECK (stage = 'canceled' OR (canceled_at IS NULL AND cancel_reason IS NULL))
);

CREATE INDEX initiatives_house_stage_idx ON initiatives (house_id, stage, created_at DESC);
CREATE INDEX initiatives_initiator_idx ON initiatives (initiator_user_id)
    WHERE initiator_user_id IS NOT NULL;

-- +goose Down
DROP TABLE initiatives;
