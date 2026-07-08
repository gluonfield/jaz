-- +goose Up
ALTER TABLE threads ADD COLUMN manual_title INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE threads DROP COLUMN manual_title;
