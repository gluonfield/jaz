-- name: ListFeed :many
-- Unread, non-archived, user-started threads whose agent turn has finished
-- (status idle, not mid-stream). The reply preview is assembled in Go from
-- LastTurnReplies (sqlite's grammar can't express the concatenation here).
SELECT id, slug, title, parent_id, status, last_attention_at_ms
FROM threads
WHERE archived = 0
  AND unread = 1
  AND status = 'idle'
  AND COALESCE(source_type, '') = ''
ORDER BY last_attention_at_ms DESC;

-- name: LastTurnReplies :many
-- Assistant reply events of the latest turn (after the last user prompt), in
-- order. A turn is often several events split around tool calls, so the card
-- concatenates the run; the last event alone drops most of the answer.
SELECT e.content, e.created_at_ms
FROM session_events e
WHERE e.thread_id = sqlc.arg(thread_id)
  AND e.type = sqlc.arg(reply_type)
  AND e.created_at_ms > COALESCE(
    (SELECT MAX(m.created_at_ms) FROM messages m WHERE m.thread_id = sqlc.arg(thread_id) AND m.role = 'user'), 0)
ORDER BY e.seq;
