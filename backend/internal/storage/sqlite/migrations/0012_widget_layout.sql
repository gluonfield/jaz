-- +goose Up
-- Board-reported layout telemetry for the current widget version (JSON from
-- the bridge: dead space, overflow, clipped elements). Cleared on publish.
ALTER TABLE widgets ADD COLUMN last_layout TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE widgets DROP COLUMN last_layout;
