-- name: ArtifactChatVisibility :one
-- Millisecond ties are ambiguous; private or unknown provenance wins.
WITH artifact AS (
  SELECT origin.content, origin.created_at_ms
  FROM session_events AS origin
  WHERE origin.thread_id = sqlc.arg(thread_id) AND origin.seq = sqlc.arg(seq) AND origin.type = 'artifact'
), message_boundary AS (
  SELECT messages.created_at_ms,
    messages.from_user = 1 AND messages.created_at_ms < (SELECT created_at_ms FROM artifact) AS visible
  FROM messages
  WHERE messages.thread_id = sqlc.arg(thread_id)
    AND messages.role = 'user'
    AND messages.created_at_ms <= (SELECT created_at_ms FROM artifact)
  ORDER BY messages.created_at_ms DESC, visible, messages.seq DESC
  LIMIT 1
), activity_boundary AS (
  SELECT session_events.created_at_ms, COALESCE(json_extract(session_events.content, '$.kind') = 'routine', 0) AS visible
  FROM session_events
  WHERE session_events.thread_id = sqlc.arg(thread_id)
    AND session_events.seq < sqlc.arg(seq)
    AND session_events.created_at_ms <= (SELECT created_at_ms FROM artifact)
    AND session_events.type = 'bot_activity'
    AND COALESCE(json_extract(session_events.content, '$.kind'), '') NOT IN ('message_sent', 'file_changed', 'file_edited')
  ORDER BY session_events.created_at_ms DESC, visible, session_events.seq DESC
  LIMIT 1
), boundaries AS (
  SELECT created_at_ms, visible FROM message_boundary
  UNION ALL
  SELECT created_at_ms, visible FROM activity_boundary
)
SELECT CAST(CASE
  WHEN json_type(artifact.content, '$.presented') IS NOT NULL THEN COALESCE(json_extract(artifact.content, '$.presented'), 0)
  ELSE COALESCE((SELECT visible FROM boundaries ORDER BY created_at_ms DESC, visible LIMIT 1), 0)
END AS INTEGER) AS visible
FROM artifact;
