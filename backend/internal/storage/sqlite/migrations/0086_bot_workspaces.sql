-- +goose Up
ALTER TABLE bots ADD COLUMN workspaces TEXT NOT NULL DEFAULT '[]';

-- +goose Down
ALTER TABLE bots DROP COLUMN workspaces;
