-- +goose Up
ALTER TABLE threads ADD COLUMN artifact_surface TEXT;

-- +goose Down
ALTER TABLE threads DROP COLUMN artifact_surface;
