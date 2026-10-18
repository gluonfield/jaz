-- name: GetLastUserMessageAt :one
-- The user's latest message of their own, queued message, answer to an agent,
-- group post or voice message. Agents write the prompts of child threads and of
-- automated sources, and the messages they send other threads, so only the
-- user's own messages in top-level chats and bot chats count. A queued message
-- counts from when the user sent it, which only the user's messages record.
SELECT CAST(MAX(
  COALESCE((
    SELECT MAX(m.created_at_ms)
    FROM messages AS m
    JOIN threads AS t ON t.id = m.thread_id
    WHERE m.role = 'user'
      AND m.from_agent = 0
      AND COALESCE(t.parent_id, '') = ''
      AND COALESCE(t.source_type, '') IN ('', 'bot')
  ), 0),
  COALESCE((
    SELECT MAX(unixepoch(json_extract(q.value, '$.sent_at'), 'subsec') * 1000)
    FROM threads AS t, json_each(CASE WHEN json_valid(t.queued_messages) THEN t.queued_messages END) AS q
  ), 0),
  COALESCE((
    SELECT created_at_ms
    FROM session_events
    WHERE type IN ('room_message', 'voice_message', 'permission_response')
      AND CASE type
        WHEN 'permission_response' THEN json_extract(permission, '$.status') = 'selected'
        ELSE 'user' IN (json_extract(content, '$.speaker'), json_extract(content, '$.role'))
      END
    ORDER BY created_at_ms DESC
    LIMIT 1
  ), 0)
) AS INTEGER);
