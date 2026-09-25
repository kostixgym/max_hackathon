-- +goose Up
CREATE TABLE registry_uploads (
    id               uuid PRIMARY KEY DEFAULT uuidv7(),
    house_id         uuid NOT NULL REFERENCES houses (id),
    version          integer NOT NULL CHECK (version > 0),
    total_area_centi bigint NOT NULL CHECK (total_area_centi > 0),
    status           text NOT NULL CHECK (status IN ('preview', 'applied')),
    report           jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(report) = 'object'),
    uploaded_by      uuid REFERENCES users (id) ON DELETE SET NULL,
    uploaded_at      timestamptz NOT NULL DEFAULT now(),
    applied_at       timestamptz,
    UNIQUE (house_id, version),
    UNIQUE (house_id, id)
);

CREATE INDEX registry_uploads_house_status_idx ON registry_uploads (house_id, status);

ALTER TABLE houses
    ADD CONSTRAINT houses_current_registry_fk
    FOREIGN KEY (id, current_registry_version)
    REFERENCES registry_uploads (house_id, version);

-- +goose Down
ALTER TABLE houses DROP CONSTRAINT houses_current_registry_fk;
DROP TABLE registry_uploads;
