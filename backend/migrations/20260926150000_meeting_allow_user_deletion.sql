-- +goose Up
-- Удаление аккаунта (решение 28) обнуляет meeting_results.finalized_by через ON DELETE SET NULL,
-- а это UPDATE, который триггер «итог записывается один раз» запрещал: пользователя, зафиксировавшего
-- итог, нельзя было удалить. Теперь разрешено единственное изменение — обнуление finalized_by;
-- сам итог по-прежнему не меняется и не удаляется.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION meeting_results_write_once() RETURNS trigger AS $$
BEGIN
    IF TG_OP = 'UPDATE' AND NEW.finalized_by IS NULL
       AND (to_jsonb(NEW) - 'finalized_by') = (to_jsonb(OLD) - 'finalized_by') THEN
        RETURN NEW;
    END IF;
    RAISE EXCEPTION 'meeting_results rows are write-once: % is not allowed', TG_OP
        USING ERRCODE = 'restrict_violation';
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION meeting_results_write_once() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'meeting_results rows are write-once: % is not allowed', TG_OP
        USING ERRCODE = 'restrict_violation';
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd
