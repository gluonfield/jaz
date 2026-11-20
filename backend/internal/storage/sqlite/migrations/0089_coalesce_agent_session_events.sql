-- +goose Up
UPDATE threads
SET event_compaction_version = 0
WHERE id IN (SELECT thread_id FROM session_events WHERE type = 'agent_session');

-- +goose Down
SELECT 1;
