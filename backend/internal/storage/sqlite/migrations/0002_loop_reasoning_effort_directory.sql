-- +goose Up
ALTER TABLE loops ADD COLUMN reasoning_effort TEXT NOT NULL DEFAULT '';
ALTER TABLE loops ADD COLUMN directory TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE loops DROP COLUMN directory;
ALTER TABLE loops DROP COLUMN reasoning_effort;
