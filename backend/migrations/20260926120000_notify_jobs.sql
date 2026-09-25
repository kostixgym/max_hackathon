-- +goose Up
-- Очередь задач модуля «Уведомления» (docs/04, модуль 8): рассылки и напоминания
-- выполняет воркер, а не обработчик запроса. PostgreSQL — единственный брокер
-- (03-arhitektura.drawio, вкладка 5C), отдельный сервис не нужен.

CREATE TABLE jobs (
    id           uuid PRIMARY KEY DEFAULT uuidv7(),
    type         text NOT NULL,
    payload      jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(payload) = 'object'),
    -- Один job на получателя: повторный запуск опроса не дублирует сообщения.
    dedup_key    text UNIQUE,
    status       text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'running', 'done', 'failed')),
    attempts     integer NOT NULL DEFAULT 0,
    max_attempts integer NOT NULL DEFAULT 5 CHECK (max_attempts > 0),
    run_at       timestamptz NOT NULL DEFAULT now(),
    last_error   text,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX jobs_poll_idx ON jobs (status, run_at);

-- +goose Down
DROP TABLE jobs;
