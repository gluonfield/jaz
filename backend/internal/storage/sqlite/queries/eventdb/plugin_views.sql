-- name: ListSessionPluginViews :many
SELECT t.id, t.title, t.status, t.updated_at_ms, t.cwd, t.workspace_id, e.seq, e.content
FROM session_events AS e
JOIN threads AS t ON t.id = e.thread_id
WHERE e.type = 'artifact'
  AND json_extract(e.content, '$.kind') = 'plugin-view'
  AND t.cwd IS NOT NULL AND t.cwd <> '';

-- name: ListThreadPluginViews :many
SELECT seq, content
FROM session_events
WHERE thread_id = sqlc.arg(thread_id)
  AND type = 'artifact'
  AND (json_extract(content, '$.kind') = 'plugin-view' OR json_extract(content, '$.file') IS NOT NULL)
ORDER BY seq;
