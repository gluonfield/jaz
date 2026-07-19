-- +goose Up
ALTER TABLE session_events ADD COLUMN projection_key TEXT NOT NULL DEFAULT '';
ALTER TABLE session_events ADD COLUMN projection_op TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE session_events DROP COLUMN projection_op;
ALTER TABLE session_events DROP COLUMN projection_key;
