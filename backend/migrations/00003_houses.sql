-- +goose Up
CREATE TABLE houses (
    id                       uuid PRIMARY KEY DEFAULT uuidv7(),
    org_id                   uuid REFERENCES organizations (id) ON DELETE SET NULL,
    address                  text NOT NULL,
    fias_id                  text,
    region                   text NOT NULL,
    timezone                 text NOT NULL DEFAULT 'Europe/Moscow',
    passport_area_centi      bigint CHECK (passport_area_centi > 0),
    current_registry_version integer CHECK (current_registry_version > 0),
    invite_slug              text NOT NULL UNIQUE,
    is_demo                  boolean NOT NULL DEFAULT false,
    created_at               timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX houses_org_idx ON houses (org_id) WHERE org_id IS NOT NULL;
CREATE UNIQUE INDEX houses_fias_unique ON houses (fias_id) WHERE fias_id IS NOT NULL;
CREATE UNIQUE INDEX houses_one_demo ON houses (is_demo) WHERE is_demo;

-- +goose Down
DROP TABLE houses;
