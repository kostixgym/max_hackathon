-- +goose Up
CREATE TABLE gis_result_entries (
    id                      uuid PRIMARY KEY DEFAULT uuidv7(),
    meeting_id              uuid NOT NULL REFERENCES meetings (id),
    agenda_item_id          uuid NOT NULL REFERENCES agenda_items (id),
    for_weight_num          bigint NOT NULL CHECK (for_weight_num >= 0),
    for_weight_den          bigint NOT NULL CHECK (for_weight_den > 0),
    against_weight_num      bigint NOT NULL CHECK (against_weight_num >= 0),
    against_weight_den      bigint NOT NULL CHECK (against_weight_den > 0),
    abstain_weight_num      bigint NOT NULL CHECK (abstain_weight_num >= 0),
    abstain_weight_den      bigint NOT NULL CHECK (abstain_weight_den > 0),
    entered_by_user_id      uuid REFERENCES users (id) ON DELETE SET NULL,
    entered_at              timestamptz NOT NULL DEFAULT now(),
    updated_at              timestamptz NOT NULL DEFAULT now(),
    UNIQUE (meeting_id, agenda_item_id)
);

-- +goose Down
DROP TABLE gis_result_entries;
