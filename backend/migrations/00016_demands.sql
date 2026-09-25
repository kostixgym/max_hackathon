-- +goose Up
CREATE TABLE demands (
    id                 uuid PRIMARY KEY DEFAULT uuidv7(),
    initiative_id      uuid NOT NULL UNIQUE REFERENCES initiatives (id) ON DELETE CASCADE,
    channel            text NOT NULL CHECK (channel IN ('paper', 'gosuslugi_dom')),
    status             text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'delivered', 'canceled')),
    support_weight_num bigint NOT NULL CHECK (support_weight_num > 0),
    support_weight_den bigint NOT NULL CHECK (support_weight_den > 0),
    created_by_user_id uuid REFERENCES users (id) ON DELETE SET NULL,
    delivered_at       timestamptz,
    uk_due_at          timestamptz,
    is_overdue         boolean NOT NULL DEFAULT false,
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now(),
    CHECK (status <> 'delivered' OR (delivered_at IS NOT NULL AND uk_due_at IS NOT NULL)),
    CHECK (delivered_at IS NULL OR uk_due_at IS NULL OR uk_due_at >= delivered_at)
);

CREATE INDEX demands_due_idx ON demands (uk_due_at)
    WHERE status = 'delivered';

-- +goose Down
DROP TABLE demands;
