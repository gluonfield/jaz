-- name: ListSessionEvents :many
SELECT
  thread_id,
  seq,
  coalesce_key,
  projection_key,
  projection_op,
  type,
  content,
  acp,
  plan,
  permission,
  created_at_ms
FROM session_events
WHERE thread_id = sqlc.arg(thread_id)
ORDER BY seq;

-- name: ListProviderSubagentEvents :many
SELECT
  thread_id,
  seq,
  projection_key,
  projection_op,
  type,
  content,
  acp,
  plan,
  permission,
  created_at_ms
FROM session_events
WHERE thread_id = sqlc.arg(thread_id)
  AND type = 'provider_subagent'
ORDER BY seq;

-- name: ListLatestACPTurn :many
WITH boundary AS (
  SELECT seq
  FROM session_events
  WHERE thread_id = sqlc.arg(thread_id)
    AND type = 'acp'
    AND json_extract(acp, '$.id') = sqlc.arg(thread_id)
    AND json_extract(acp, '$.state') IN ('idle', 'failed', 'cancelled')
  ORDER BY seq DESC
  LIMIT 1 OFFSET 1
)
SELECT
  events.thread_id,
  events.seq,
  events.projection_key,
  events.projection_op,
  events.type,
  events.content,
  events.acp,
  events.plan,
  events.permission,
  events.created_at_ms
FROM session_events AS events
WHERE events.thread_id = sqlc.arg(thread_id)
  AND events.seq > COALESCE((SELECT boundary.seq FROM boundary), 0)
ORDER BY events.seq;

-- name: ListSessionEventCompactionPage :many
SELECT
  thread_id,
  seq,
  coalesce_key,
  projection_key,
  projection_op,
  type,
  content,
  acp,
  plan,
  permission,
  created_at_ms
FROM session_events
WHERE thread_id = sqlc.arg(thread_id)
  AND seq > sqlc.arg(after_seq)
ORDER BY seq
LIMIT sqlc.arg(limit_count);

-- name: ListSessionEventPage :many
SELECT
  thread_id,
  seq,
  projection_key,
  projection_op,
  type,
  content,
  acp,
  plan,
  permission,
  created_at_ms
FROM session_events
WHERE thread_id = sqlc.arg(thread_id)
  AND (sqlc.arg(before_seq) = 0 OR seq < sqlc.arg(before_seq))
ORDER BY seq DESC
LIMIT sqlc.arg(limit_count);

-- name: ListSessionEventPageSizes :many
SELECT
  seq,
  LENGTH(CAST(content AS BLOB))
    + COALESCE(LENGTH(CAST(acp AS BLOB)), 0)
    + COALESCE(LENGTH(CAST(plan AS BLOB)), 0)
    + COALESCE(LENGTH(CAST(permission AS BLOB)), 0) AS bytes
FROM session_events
WHERE thread_id = sqlc.arg(thread_id)
  AND (sqlc.arg(before_seq) = 0 OR seq < sqlc.arg(before_seq))
ORDER BY seq DESC
LIMIT sqlc.arg(limit_count);

-- name: LatestSessionEventSeq :one
SELECT CAST(COALESCE(MAX(seq), 0) AS INTEGER) AS seq
FROM session_events
WHERE thread_id = sqlc.arg(thread_id);

-- name: ListSessionEventsAfter :many
SELECT
  thread_id,
  seq,
  projection_key,
  projection_op,
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
  projection_key,
  projection_op,
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

-- name: NextSessionEventCompaction :one
SELECT id
FROM threads
WHERE event_compaction_version = 0
  AND status <> sqlc.arg(running_status)
ORDER BY event_revision DESC, updated_at_ms
LIMIT 1;

-- name: HasPendingSessionEventCompaction :one
SELECT EXISTS (
  SELECT 1
  FROM threads
  WHERE event_compaction_version = 0
    AND status <> sqlc.arg(running_status)
);

-- name: AdvanceSessionEventRevision :exec
UPDATE threads
SET event_revision = event_revision + 1,
    event_compaction_version = CASE
      WHEN sqlc.arg(compaction_pending) THEN 0
      ELSE event_compaction_version
    END
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
  projection_key,
  projection_op,
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
  sqlc.arg(projection_key),
  sqlc.arg(projection_op),
  sqlc.arg(type),
  sqlc.arg(content),
  sqlc.narg(acp),
  sqlc.narg(plan),
  sqlc.narg(permission),
  sqlc.arg(created_at_ms)
)
ON CONFLICT(thread_id, seq) DO UPDATE SET
  coalesce_key = excluded.coalesce_key,
  projection_key = excluded.projection_key,
  projection_op = excluded.projection_op,
  type = excluded.type,
  content = excluded.content,
  acp = excluded.acp,
  plan = excluded.plan,
  permission = excluded.permission,
  created_at_ms = excluded.created_at_ms;

-- name: DeleteSessionEventByCoalesceKey :execrows
DELETE FROM session_events
WHERE thread_id = sqlc.arg(thread_id) AND coalesce_key = sqlc.arg(coalesce_key);

-- name: GetSessionEventByCoalesceKey :one
SELECT
  thread_id,
  seq,
  projection_key,
  projection_op,
  type,
  content,
  acp,
  plan,
  permission,
  created_at_ms
FROM session_events
WHERE thread_id = sqlc.arg(thread_id) AND coalesce_key = sqlc.arg(coalesce_key);

-- name: SetSessionEventCoalesceKey :exec
UPDATE session_events
SET coalesce_key = sqlc.arg(coalesce_key)
WHERE thread_id = sqlc.arg(thread_id) AND seq = sqlc.arg(seq);

-- name: SetSessionEventProjection :exec
UPDATE session_events
SET coalesce_key = sqlc.arg(coalesce_key),
    projection_key = sqlc.arg(projection_key),
    projection_op = sqlc.arg(projection_op)
WHERE thread_id = sqlc.arg(thread_id) AND seq = sqlc.arg(seq);

-- name: DeleteSessionEvent :exec
DELETE FROM session_events
WHERE thread_id = sqlc.arg(thread_id) AND seq = sqlc.arg(seq);

-- name: DeleteSessionEvents :exec
DELETE FROM session_events
WHERE thread_id = sqlc.arg(thread_id) AND seq IN (sqlc.slice('seqs'));
