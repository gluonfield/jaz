-- name: ListMessagesByThread :many
SELECT
  thread_id,
  seq,
  role,
  content,
  reasoning,
  blocks,
  created_at_ms
FROM messages
WHERE thread_id = sqlc.arg(thread_id)
ORDER BY seq;

-- name: DeleteMessagesByThread :exec
DELETE FROM messages
WHERE thread_id = sqlc.arg(thread_id);

-- name: InsertMessage :exec
INSERT INTO messages (
  thread_id,
  seq,
  role,
  content,
  reasoning,
  blocks,
  created_at_ms
) VALUES (
  sqlc.arg(thread_id),
  sqlc.arg(seq),
  sqlc.arg(role),
  sqlc.arg(content),
  sqlc.narg(reasoning),
  sqlc.arg(blocks),
  sqlc.arg(created_at_ms)
);
