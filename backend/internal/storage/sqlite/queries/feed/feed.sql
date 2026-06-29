-- name: ListFeed :many
-- Unread user-started threads, with their newest message for display. The
-- assistant-role guard drops threads whose last message is the user's own.
SELECT
  t.id,
  t.slug,
  t.title,
  t.parent_id,
  t.status,
  m.role AS message_role,
  m.content AS message_content,
  m.blocks AS message_blocks,
  m.created_at_ms AS message_created_at_ms
FROM threads t
JOIN messages m
  ON m.thread_id = t.id
 AND m.seq = (SELECT MAX(seq) FROM messages m2 WHERE m2.thread_id = t.id)
WHERE t.archived = 0
  AND t.unread = 1
  AND COALESCE(t.source_type, '') = ''
  AND m.role = 'assistant'
ORDER BY m.created_at_ms DESC;
