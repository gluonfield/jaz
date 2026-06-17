-- +goose Up
ALTER TABLE devices ADD COLUMN platform TEXT NOT NULL DEFAULT '';
ALTER TABLE devices ADD COLUMN device_family TEXT NOT NULL DEFAULT '';
ALTER TABLE devices ADD COLUMN model_identifier TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE devices DROP COLUMN model_identifier;
ALTER TABLE devices DROP COLUMN device_family;
ALTER TABLE devices DROP COLUMN platform;
