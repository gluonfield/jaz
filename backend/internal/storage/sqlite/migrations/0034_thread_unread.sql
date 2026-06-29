-- +goose Up
ALTER TABLE threads ADD COLUMN unread INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE threads DROP COLUMN unread;
