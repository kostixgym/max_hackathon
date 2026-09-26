-- +goose Up
-- Позиция long polling бота: сохраняется после каждой обработанной пачки апдейтов,
-- поэтому перезапуск процесса не теряет и не дублирует события. Владелец — модуль
-- «Уведомления» (как и очередь): платформенный поллер работает с таблицей через
-- его интерфейс, не трогая SQL (docs/04, принцип 8).

CREATE TABLE bot_markers (
    bot_user_id bigint PRIMARY KEY,
    marker      bigint NOT NULL DEFAULT 0 CHECK (marker >= 0),
    updated_at  timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE bot_markers;
