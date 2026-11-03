-- +goose Up
ALTER TABLE bot_commitments ADD COLUMN name TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE bot_commitments DROP COLUMN name;
