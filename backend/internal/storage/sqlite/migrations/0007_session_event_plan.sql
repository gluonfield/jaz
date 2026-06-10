-- +goose Up
ALTER TABLE session_events ADD COLUMN plan TEXT;

-- +goose Down
ALTER TABLE session_events DROP COLUMN plan;
