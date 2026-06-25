-- +goose Up
ALTER TABLE threads ADD COLUMN runtime_capabilities TEXT NOT NULL DEFAULT '{}';

-- +goose Down
ALTER TABLE threads DROP COLUMN runtime_capabilities;
