-- name: ListSessionEvents :many
SELECT
  thread_id,
  seq,
  type,
  content,
  acp,
  plan,
  permission,
  created_at_ms
FROM session_events
WHERE thread_id = sqlc.arg(thread_id)
ORDER BY seq;

-- name: ListSessionEventsAfter :many
SELECT
  thread_id,
  seq,
  type,
  content,
  acp,
  plan,
  permission,
  created_at_ms
FROM session_events
WHERE thread_id = sqlc.arg(thread_id)
  AND seq > sqlc.arg(after_seq)
ORDER BY seq;

-- name: ListSessionEventsAfterTime :many
SELECT
  thread_id,
  seq,
  type,
  content,
  acp,
  plan,
  permission,
  created_at_ms
FROM session_events
WHERE thread_id = sqlc.arg(thread_id)
  AND created_at_ms > sqlc.arg(after_ms)
ORDER BY seq;

-- name: NextSessionEventSeq :one
SELECT COALESCE(MAX(seq), 0) + 1 AS seq
FROM session_events
WHERE thread_id = sqlc.arg(thread_id);

-- name: GetSessionEventCompactionState :one
SELECT event_compaction_version, event_revision, status
FROM threads
WHERE id = sqlc.arg(thread_id);

-- name: NextLegacySessionEventThread :one
SELECT id
FROM threads
WHERE event_compaction_version = 0
  AND status <> sqlc.arg(running_status)
ORDER BY event_revision DESC, updated_at_ms
LIMIT 1;

-- name: HasLegacySessionEventThreads :one
SELECT EXISTS (
  SELECT 1
  FROM threads
  WHERE event_compaction_version = 0
);

-- name: AdvanceSessionEventRevision :exec
UPDATE threads
SET event_revision = event_revision + 1
WHERE id = sqlc.arg(thread_id);

-- name: CompleteSessionEventCompaction :execrows
UPDATE threads
SET event_compaction_version = 1
WHERE id = sqlc.arg(thread_id)
  AND event_compaction_version = 0
  AND event_revision = sqlc.arg(event_revision)
  AND status <> sqlc.arg(running_status);

-- name: SkipSessionEventCompaction :execrows
UPDATE threads
SET event_compaction_version = 2
WHERE id = sqlc.arg(thread_id)
  AND event_compaction_version = 0
  AND event_revision = sqlc.arg(event_revision)
  AND status <> sqlc.arg(running_status);

-- name: UpsertSessionEvent :exec
INSERT INTO session_events (
  thread_id,
  seq,
  coalesce_key,
  type,
  content,
  acp,
  plan,
  permission,
  created_at_ms
) VALUES (
  sqlc.arg(thread_id),
  sqlc.arg(seq),
  sqlc.arg(coalesce_key),
  sqlc.arg(type),
  sqlc.arg(content),
  sqlc.narg(acp),
  sqlc.narg(plan),
  sqlc.narg(permission),
  sqlc.arg(created_at_ms)
)
ON CONFLICT(thread_id, seq) DO UPDATE SET
  coalesce_key = excluded.coalesce_key,
  type = excluded.type,
  content = excluded.content,
  acp = excluded.acp,
  plan = excluded.plan,
  permission = excluded.permission,
  created_at_ms = excluded.created_at_ms;

-- name: DeleteSessionEventByCoalesceKey :exec
DELETE FROM session_events
WHERE thread_id = sqlc.arg(thread_id) AND coalesce_key = sqlc.arg(coalesce_key);

-- name: SetSessionEventCoalesceKey :exec
UPDATE session_events
SET coalesce_key = sqlc.arg(coalesce_key)
WHERE thread_id = sqlc.arg(thread_id) AND seq = sqlc.arg(seq);

-- name: DeleteSessionEvent :exec
DELETE FROM session_events
WHERE thread_id = sqlc.arg(thread_id) AND seq = sqlc.arg(seq);
