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

-- name: ListUserMessageBoundaries :many
SELECT seq, created_at_ms
FROM messages
WHERE thread_id = sqlc.arg(thread_id)
  AND role = 'user'
  AND (sqlc.arg(before_seq) = 0 OR seq < sqlc.arg(before_seq))
ORDER BY seq DESC
LIMIT sqlc.arg(limit_count);

-- name: GetMessageTime :one
SELECT created_at_ms
FROM messages
WHERE thread_id = sqlc.arg(thread_id)
  AND seq = sqlc.arg(seq);

-- name: LatestUserMessageBeforeEvent :one
WITH boundary AS (
  SELECT boundary_messages.seq
  FROM messages AS boundary_messages
  WHERE boundary_messages.thread_id = sqlc.arg(thread_id)
    AND boundary_messages.created_at_ms > sqlc.arg(created_at_ms)
  ORDER BY boundary_messages.seq
  LIMIT 1
)
SELECT messages.seq, messages.created_at_ms
FROM messages
WHERE messages.thread_id = sqlc.arg(thread_id)
  AND messages.role = 'user'
  AND ((SELECT seq FROM boundary) IS NULL OR messages.seq < (SELECT seq FROM boundary))
ORDER BY messages.seq DESC
LIMIT 1;

-- name: NextMessageSeq :one
SELECT COALESCE(MAX(seq), 0) + 1 AS seq
FROM messages
WHERE thread_id = sqlc.arg(thread_id);

-- name: ListMessagesByThreadRange :many
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
  AND seq >= sqlc.arg(start_seq)
  AND (sqlc.arg(before_seq) = 0 OR seq < sqlc.arg(before_seq))
ORDER BY seq;

-- name: ListMessageRangeSizes :many
SELECT
  seq,
  role,
  created_at_ms,
  LENGTH(CAST(content AS BLOB))
    + COALESCE(LENGTH(CAST(reasoning AS BLOB)), 0)
    + LENGTH(CAST(blocks AS BLOB)) AS bytes
FROM messages
WHERE thread_id = sqlc.arg(thread_id)
  AND seq >= sqlc.arg(start_seq)
  AND (sqlc.arg(before_seq) = 0 OR seq < sqlc.arg(before_seq))
ORDER BY seq DESC;

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
