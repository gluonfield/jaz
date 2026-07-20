-- +goose Up
CREATE INDEX idx_session_events_provider_subagents
ON session_events(thread_id, seq)
WHERE type = 'provider_subagent';

CREATE INDEX idx_threads_overview_children
ON threads(parent_id, last_attention_at_ms DESC, id)
WHERE runtime = 'acp' AND COALESCE(source_type, '') = '';

-- +goose Down
DROP INDEX idx_threads_overview_children;
DROP INDEX idx_session_events_provider_subagents;
