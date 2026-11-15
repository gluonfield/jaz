-- name: ListSessionFolders :many
SELECT id, title, status, updated_at_ms, cwd
FROM threads
WHERE cwd IS NOT NULL AND cwd <> '';
