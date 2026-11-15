-- +goose Up
CREATE INDEX idx_session_events_artifacts
ON session_events(thread_id, seq)
WHERE type = 'artifact';

-- +goose Down
DROP INDEX idx_session_events_artifacts;
