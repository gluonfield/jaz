-- +goose Up
-- When the thread's last successful turn started: its agent was shown what
-- the thread held then.
ALTER TABLE threads ADD COLUMN completed_turn_started_at_ms INTEGER NOT NULL DEFAULT 0;

CREATE INDEX idx_session_events_bot_activity
ON session_events(thread_id, created_at_ms)
WHERE type = 'bot_activity';

-- +goose Down
DROP INDEX idx_session_events_bot_activity;
ALTER TABLE threads DROP COLUMN completed_turn_started_at_ms;
