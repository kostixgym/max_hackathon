-- +goose Up
CREATE TABLE organizations (
    id         uuid PRIMARY KEY DEFAULT uuidv7(),
    type       text NOT NULL CHECK (type IN ('uk', 'tszh', 'zhsk')),
    name       text NOT NULL,
    inn        text,
    contacts   jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(contacts) = 'object'),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX organizations_inn_unique
    ON organizations (inn)
    WHERE inn IS NOT NULL;

-- +goose Down
DROP TABLE organizations;
