-- +goose Up
ALTER TABLE loops ADD COLUMN memory_path TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE loops DROP COLUMN memory_path;
