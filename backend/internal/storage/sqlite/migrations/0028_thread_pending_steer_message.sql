-- +goose Up
ALTER TABLE threads ADD COLUMN pending_steer_message TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE threads DROP COLUMN pending_steer_message;
