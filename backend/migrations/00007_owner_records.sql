-- +goose Up
CREATE TABLE owner_records (
    id                 uuid PRIMARY KEY DEFAULT uuidv7(),
    owner_id           uuid NOT NULL REFERENCES owners (id),
    registry_upload_id uuid NOT NULL REFERENCES registry_uploads (id) ON DELETE CASCADE,
    full_name          text NOT NULL,
    share_num          bigint NOT NULL CHECK (share_num > 0),
    share_den          bigint NOT NULL CHECK (share_den > 0),
    weight_num         bigint NOT NULL CHECK (weight_num > 0),
    weight_den         bigint NOT NULL CHECK (weight_den > 0),
    owner_kind         text NOT NULL CHECK (owner_kind IN ('person', 'organization', 'municipality')),
    phone_hmac         bytea,
    CHECK (share_num <= share_den),
    UNIQUE (owner_id, registry_upload_id)
);

CREATE INDEX owner_records_upload_idx ON owner_records (registry_upload_id);
CREATE INDEX owner_records_phone_idx ON owner_records (phone_hmac) WHERE phone_hmac IS NOT NULL;

-- +goose Down
DROP TABLE owner_records;
