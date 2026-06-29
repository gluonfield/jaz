-- name: ListFeed :many
-- Unread, non-archived, user-started threads with the agent's latest reply for
-- the card preview. Assistant text lives in session_events (the caller passes the
-- reply event type), not in the messages table which holds user turns.
SELECT
  t.id,
  t.slug,
  t.title,
  t.parent_id,
  t.status,
  e.content AS message_content,
  COALESCE(e.created_at_ms, t.last_attention_at_ms) AS message_created_at_ms
FROM threads t
LEFT JOIN session_events e
  ON e.thread_id = t.id
 AND e.seq = (SELECT MAX(e2.seq) FROM session_events e2 WHERE e2.thread_id = t.id AND e2.type = sqlc.arg(reply_type))
WHERE t.archived = 0
  AND t.unread = 1
  AND COALESCE(t.source_type, '') = ''
ORDER BY message_created_at_ms DESC;
