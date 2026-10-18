-- +goose Up
-- A message an agent wrote into another thread, as with send_message_to_thread,
-- has the user's role without the user writing it.
ALTER TABLE messages ADD COLUMN from_agent INTEGER NOT NULL DEFAULT 0;

-- Finds the user's latest group post, voice message or answer to an agent
-- without reading every session event.
DROP INDEX idx_session_events_user_speech;
CREATE INDEX idx_session_events_user_acts
ON session_events(created_at_ms)
WHERE type IN ('room_message', 'voice_message', 'permission_response');

-- +goose Down
DROP INDEX idx_session_events_user_acts;
CREATE INDEX idx_session_events_user_speech
ON session_events(created_at_ms)
WHERE type IN ('room_message', 'voice_message');
ALTER TABLE messages DROP COLUMN from_agent;
