-- +goose Up
-- Carry the session's source type onto each usage event so usage can be split
-- by activity (chat vs loops vs memory dream/search vs browser agent). Existing
-- rows predate the column; '' reads as chat, matching interactive sessions.
ALTER TABLE usage_events ADD COLUMN source_type TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE usage_events DROP COLUMN source_type;
