-- name: ListSessionPresentations :many
SELECT t.id, t.title, t.status, t.updated_at_ms, t.cwd, e.seq, e.content
FROM session_events AS e
JOIN threads AS t ON t.id = e.thread_id
WHERE e.type = 'artifact' AND t.cwd IS NOT NULL AND t.cwd <> '';
