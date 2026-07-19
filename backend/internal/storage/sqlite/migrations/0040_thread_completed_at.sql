-- +goose Up
ALTER TABLE threads ADD COLUMN last_completed_at_ms INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE threads DROP COLUMN last_completed_at_ms;
