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

-- name: NextSessionEventSeq :one
SELECT COALESCE(MAX(seq), 0) + 1 AS seq
FROM session_events
WHERE thread_id = sqlc.arg(thread_id);

-- name: UpsertSessionEvent :exec
INSERT INTO session_events (
  thread_id,
  seq,
  type,
  content,
  acp,
  plan,
  permission,
  created_at_ms
) VALUES (
  sqlc.arg(thread_id),
  sqlc.arg(seq),
  sqlc.arg(type),
  sqlc.arg(content),
  sqlc.narg(acp),
  sqlc.narg(plan),
  sqlc.narg(permission),
  sqlc.arg(created_at_ms)
)
ON CONFLICT(thread_id, seq) DO UPDATE SET
  type = excluded.type,
  content = excluded.content,
  acp = excluded.acp,
  plan = excluded.plan,
  permission = excluded.permission,
  created_at_ms = excluded.created_at_ms;
