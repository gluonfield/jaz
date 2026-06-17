-- +goose Up
ALTER TABLE devices ADD COLUMN public_key TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE devices DROP COLUMN public_key;
