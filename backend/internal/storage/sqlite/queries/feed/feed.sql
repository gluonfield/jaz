-- name: ListFeed :many
SELECT id, slug, title, parent_id, status, last_attention_at_ms
FROM threads
WHERE archived = 0
  AND unread = 1
  AND status = 'idle'
  AND COALESCE(source_type, '') = ''
ORDER BY last_attention_at_ms DESC;

-- name: LastTurnReplies :many
-- A turn splits into several events around tool calls; the caller concatenates
-- them in Go because sqlite's grammar can't express the ordered concatenation.
SELECT e.content, e.created_at_ms
FROM session_events e
WHERE e.thread_id = sqlc.arg(thread_id)
  AND e.type = sqlc.arg(reply_type)
  AND e.created_at_ms > COALESCE(
    (SELECT MAX(m.created_at_ms) FROM messages m WHERE m.thread_id = sqlc.arg(thread_id) AND m.role = 'user'), 0)
ORDER BY e.seq;
