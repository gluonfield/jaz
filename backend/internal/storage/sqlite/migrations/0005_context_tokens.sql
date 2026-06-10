-- +goose Up
ALTER TABLE threads ADD COLUMN context_tokens INTEGER NOT NULL DEFAULT 0;
ALTER TABLE threads ADD COLUMN context_window_tokens INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE threads DROP COLUMN context_window_tokens;
ALTER TABLE threads DROP COLUMN context_tokens;
