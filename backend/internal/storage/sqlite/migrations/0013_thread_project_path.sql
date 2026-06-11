-- +goose Up
ALTER TABLE threads ADD COLUMN project_path TEXT;

-- +goose Down
ALTER TABLE threads DROP COLUMN project_path;
