-- +goose Up
ALTER TABLE boards ADD COLUMN font_scale REAL NOT NULL DEFAULT 1;

-- +goose Down
ALTER TABLE boards DROP COLUMN font_scale;
