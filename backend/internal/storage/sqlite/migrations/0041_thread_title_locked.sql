-- +goose Up
ALTER TABLE threads ADD COLUMN title_locked INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE threads DROP COLUMN title_locked;
