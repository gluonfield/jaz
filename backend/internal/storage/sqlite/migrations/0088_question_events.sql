-- +goose Up
CREATE INDEX idx_session_events_permissions
ON session_events(thread_id, seq)
WHERE type IN ('permission_request', 'permission_response');

-- +goose Down
DROP INDEX idx_session_events_permissions;
