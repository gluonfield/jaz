-- +goose Up
CREATE INDEX idx_session_events_chat
ON session_events(thread_id, seq DESC)
WHERE type IN ('acp', 'room_message', 'bot_activity', 'mcp_app', 'permission_request', 'permission_response', 'loop_created', 'agent_switch');

-- +goose Down
DROP INDEX idx_session_events_chat;
