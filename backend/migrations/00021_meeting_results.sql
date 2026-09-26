-- +goose Up
CREATE TABLE meeting_results (
    id                 uuid PRIMARY KEY DEFAULT uuidv7(),
    meeting_id         uuid NOT NULL REFERENCES meetings (id),
    agenda_item_id     uuid NOT NULL REFERENCES agenda_items (id),
    for_weight_num     bigint NOT NULL CHECK (for_weight_num >= 0),
    for_weight_den     bigint NOT NULL CHECK (for_weight_den > 0),
    against_weight_num bigint NOT NULL CHECK (against_weight_num >= 0),
    against_weight_den bigint NOT NULL CHECK (against_weight_den > 0),
    abstain_weight_num bigint NOT NULL CHECK (abstain_weight_num >= 0),
    abstain_weight_den bigint NOT NULL CHECK (abstain_weight_den > 0),
    total_weight_num   bigint NOT NULL CHECK (total_weight_num > 0),
    total_weight_den   bigint NOT NULL CHECK (total_weight_den > 0),
    majority_rule      text NOT NULL CHECK (majority_rule IN (
        'majority_of_participants',
        'more_than_half_of_all',
        'two_thirds_of_all'
    )),
    accepted           boolean NOT NULL,
    finalized_by       uuid REFERENCES users (id) ON DELETE SET NULL,
    finalized_at       timestamptz NOT NULL DEFAULT now(),
    UNIQUE (meeting_id, agenda_item_id)
);

-- Итог по вопросу записывается один раз при фиксации и дальше только читается (решение 61):
-- протокол строится из этих строк, правка задним числом переписала бы историю.
-- +goose StatementBegin
CREATE FUNCTION meeting_results_write_once() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'meeting_results rows are write-once: % is not allowed', TG_OP
        USING ERRCODE = 'restrict_violation';
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER meeting_results_write_once
    BEFORE UPDATE OR DELETE ON meeting_results
    FOR EACH ROW EXECUTE FUNCTION meeting_results_write_once();

-- +goose Down
DROP TABLE meeting_results;
DROP FUNCTION meeting_results_write_once();
