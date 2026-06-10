-- +goose Up
ALTER TABLE loops ADD COLUMN model_provider TEXT NOT NULL DEFAULT '';
ALTER TABLE loops ADD COLUMN model TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE loops DROP COLUMN model;
ALTER TABLE loops DROP COLUMN model_provider;
