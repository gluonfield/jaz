-- name: ListFeed :many
SELECT id, slug, title, parent_id, status, last_attention_at_ms
FROM threads
WHERE archived = 0
  AND unread = 1
  AND status = 'idle'
  AND COALESCE(source_type, '') = ''
ORDER BY last_attention_at_ms DESC;

-- name: LastUserPromptAt :one
SELECT CAST(COALESCE(MAX(created_at_ms), 0) AS INTEGER) AS at_ms
FROM messages
WHERE thread_id = sqlc.arg(thread_id) AND role = 'user';
