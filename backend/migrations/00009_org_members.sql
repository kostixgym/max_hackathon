-- +goose Up
CREATE TABLE org_members (
    user_id    uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    org_id     uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    role       text NOT NULL CHECK (role IN ('admin', 'operator')),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, org_id)
);

CREATE INDEX org_members_org_idx ON org_members (org_id);

-- +goose Down
DROP TABLE org_members;
