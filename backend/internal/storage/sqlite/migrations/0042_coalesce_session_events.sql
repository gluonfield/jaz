-- +goose Up
ALTER TABLE session_events ADD COLUMN coalesce_key TEXT NOT NULL DEFAULT '';
ALTER TABLE threads ADD COLUMN event_compaction_version INTEGER NOT NULL DEFAULT 1;
ALTER TABLE threads ADD COLUMN event_revision INTEGER NOT NULL DEFAULT 0;

UPDATE threads
SET event_compaction_version = 0,
    event_revision = (
      SELECT COUNT(*)
      FROM session_events
      WHERE session_events.thread_id = threads.id
    )
WHERE EXISTS (
  SELECT 1
  FROM session_events
  WHERE session_events.thread_id = threads.id
);

CREATE UNIQUE INDEX idx_session_events_coalesce
ON session_events(thread_id, coalesce_key)
WHERE coalesce_key <> '';

CREATE INDEX idx_threads_event_compaction_pending
ON threads(event_revision DESC, updated_at_ms)
WHERE event_compaction_version = 0;

-- +goose Down
DROP INDEX idx_threads_event_compaction_pending;
DROP INDEX idx_session_events_coalesce;
ALTER TABLE threads DROP COLUMN event_revision;
ALTER TABLE threads DROP COLUMN event_compaction_version;
ALTER TABLE session_events DROP COLUMN coalesce_key;
