-- +goose Up
DROP INDEX IF EXISTS idx_session_events_thread_seq;

-- +goose Down
CREATE INDEX IF NOT EXISTS idx_session_events_thread_seq ON session_events(thread_id, seq);
