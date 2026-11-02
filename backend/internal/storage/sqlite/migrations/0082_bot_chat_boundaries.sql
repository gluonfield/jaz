-- +goose Up
DROP INDEX idx_session_events_chat;
CREATE INDEX idx_session_events_chat
ON session_events(thread_id, seq DESC)
WHERE type IN ('acp', 'room_message', 'bot_activity', 'mcp_app', 'artifact', 'permission_request', 'permission_response', 'loop_created', 'agent_switch');

CREATE INDEX idx_session_events_chat_entries
ON session_events(thread_id, seq DESC)
WHERE type IN ('room_message', 'mcp_app', 'artifact', 'permission_request', 'loop_created', 'agent_switch')
  OR (type = 'bot_activity' AND json_extract(CASE WHEN json_valid(content) THEN content END, '$.kind') = 'unreachable');

-- +goose Down
DROP INDEX idx_session_events_chat_entries;
DROP INDEX idx_session_events_chat;
CREATE INDEX idx_session_events_chat
ON session_events(thread_id, seq DESC)
WHERE type IN ('acp', 'room_message', 'bot_activity', 'mcp_app', 'permission_request', 'permission_response', 'loop_created', 'agent_switch');
