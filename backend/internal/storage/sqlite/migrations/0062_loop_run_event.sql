-- +goose Up
ALTER TABLE loop_runs ADD COLUMN event TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE loop_runs DROP COLUMN event;
