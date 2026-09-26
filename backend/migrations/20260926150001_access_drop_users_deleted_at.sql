-- +goose Up
-- Удаление аккаунта — удаление строки users: привязки и голоса опроса уходят каскадом, остальные
-- ссылки обнуляются (решение 28). Флаг deleted_at этому противоречил и нигде не использовался.
ALTER TABLE users DROP COLUMN deleted_at;

-- +goose Down
ALTER TABLE users ADD COLUMN deleted_at timestamptz;
