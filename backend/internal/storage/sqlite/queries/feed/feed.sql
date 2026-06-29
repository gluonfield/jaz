-- name: ListFeed :many
-- Every unarchived thread whose newest message is an unseen assistant reply,
-- with that message attached. Restricting to assistant-authored last messages is
-- what "threads I need to respond to" means: a thread whose last message is the
-- user's own is waiting on the agent, not on you. The correlated MAX(seq) is an
-- index-only seek on the (thread_id, seq) primary key, so this is one round trip.
SELECT
  t.id,
  t.slug,
  t.title,
  t.parent_id,
  t.status,
  m.role AS message_role,
  m.content AS message_content,
  m.reasoning AS message_reasoning,
  m.blocks AS message_blocks,
  m.created_at_ms AS message_created_at_ms
FROM threads t
JOIN messages m
  ON m.thread_id = t.id
 AND m.seq = (SELECT MAX(seq) FROM messages m2 WHERE m2.thread_id = t.id)
WHERE t.archived = 0
  AND m.role = 'assistant'
  AND m.created_at_ms > t.last_seen_at_ms
ORDER BY m.created_at_ms DESC;
