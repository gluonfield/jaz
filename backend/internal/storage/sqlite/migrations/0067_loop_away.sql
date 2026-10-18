-- +goose Up
ALTER TABLE loops ADD COLUMN keep_running INTEGER NOT NULL DEFAULT 0;

-- Finds the user's latest group post or voice message without reading every
-- session event.
CREATE INDEX idx_session_events_user_speech
ON session_events(created_at_ms)
WHERE type IN ('room_message', 'voice_message');

-- +goose Down
DROP INDEX idx_session_events_user_speech;
ALTER TABLE loops DROP COLUMN keep_running;
