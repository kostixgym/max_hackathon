-- +goose Up
CREATE TABLE audit_logs (
    id            uuid PRIMARY KEY DEFAULT uuidv7(),
    actor_user_id uuid REFERENCES users (id) ON DELETE SET NULL,
    action        text NOT NULL,
    entity_type   text NOT NULL,
    entity_id     uuid,
    changes       jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(changes) = 'object'),
    reason        text,
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX audit_logs_entity_idx ON audit_logs (entity_type, entity_id, created_at DESC);
CREATE INDEX audit_logs_actor_idx ON audit_logs (actor_user_id, created_at DESC)
    WHERE actor_user_id IS NOT NULL;
CREATE INDEX audit_logs_created_at_idx ON audit_logs (created_at);

-- +goose Down
DROP TABLE audit_logs;
