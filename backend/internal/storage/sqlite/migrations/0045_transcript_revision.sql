-- +goose Up
ALTER TABLE threads ADD COLUMN transcript_revision INTEGER NOT NULL DEFAULT 1;

-- +goose Down
ALTER TABLE threads DROP COLUMN transcript_revision;
