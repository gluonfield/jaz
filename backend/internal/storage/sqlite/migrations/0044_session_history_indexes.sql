-- +goose Up
CREATE INDEX idx_messages_thread_role_seq
ON messages(thread_id, role, seq DESC);

CREATE INDEX idx_session_events_thread_created
ON session_events(thread_id, created_at_ms, seq);

-- +goose Down
DROP INDEX idx_session_events_thread_created;
DROP INDEX idx_messages_thread_role_seq;
