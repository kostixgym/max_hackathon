-- +goose Up
CREATE TABLE memberships (
    id               uuid PRIMARY KEY DEFAULT uuidv7(),
    user_id          uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    premise_id       uuid NOT NULL REFERENCES premises (id),
    owner_id         uuid REFERENCES owners (id),
    role             text NOT NULL CHECK (role IN ('guest', 'resident', 'owner')),
    method           text CHECK (method IN ('phone', 'account', 'uk_manual', 'demo')),
    status           text NOT NULL CHECK (status IN ('pending', 'verified', 'rejected', 'revoked')),
    verified_by      uuid REFERENCES users (id) ON DELETE SET NULL,
    verified_at      timestamptz,
    rejected_at      timestamptz,
    rejection_reason text,
    created_at       timestamptz NOT NULL DEFAULT now(),
    UNIQUE (user_id, premise_id),
    CHECK ((role = 'owner') = (owner_id IS NOT NULL)),
    CHECK (status = 'rejected' OR rejection_reason IS NULL)
);

CREATE INDEX memberships_premise_idx ON memberships (premise_id);
CREATE INDEX memberships_owner_idx ON memberships (owner_id) WHERE owner_id IS NOT NULL;
CREATE UNIQUE INDEX memberships_one_verified_owner
    ON memberships (owner_id)
    WHERE role = 'owner' AND status = 'verified';

-- +goose Down
DROP TABLE memberships;
