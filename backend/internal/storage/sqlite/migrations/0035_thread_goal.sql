-- +goose Up
ALTER TABLE threads ADD COLUMN goal TEXT NOT NULL DEFAULT '{}';

-- +goose Down
ALTER TABLE threads DROP COLUMN goal;
