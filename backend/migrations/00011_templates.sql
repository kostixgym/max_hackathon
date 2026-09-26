-- +goose Up
CREATE TABLE templates (
    id            uuid PRIMARY KEY DEFAULT uuidv7(),
    code          text NOT NULL,
    version       integer NOT NULL CHECK (version > 0),
    name          text NOT NULL,
    description   text,
    params_schema jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(params_schema) = 'object'),
    ui_schema     jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(ui_schema) = 'object'),
    created_at    timestamptz NOT NULL DEFAULT now(),
    UNIQUE (code, version)
);

CREATE INDEX templates_code_version_idx ON templates (code, version DESC);

-- +goose Down
DROP TABLE templates;
