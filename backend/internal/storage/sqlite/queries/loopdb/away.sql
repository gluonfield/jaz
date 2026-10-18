-- name: GetLastUserMessageAt :one
-- Agents write the prompts of child threads and of automated sources, so only
-- top-level chats and bot chats hold messages the user typed.
SELECT CAST(COALESCE(MAX(m.created_at_ms), 0) AS INTEGER)
FROM messages AS m
JOIN threads AS t ON t.id = m.thread_id
WHERE m.role = 'user'
  AND COALESCE(t.parent_id, '') = ''
  AND COALESCE(t.source_type, '') IN ('', 'bot');

-- name: GetLastUserSpeechAt :one
SELECT created_at_ms
FROM session_events
WHERE type IN ('room_message', 'voice_message')
  AND 'user' IN (json_extract(content, '$.speaker'), json_extract(content, '$.role'))
ORDER BY created_at_ms DESC
LIMIT 1;
