-- name: LoadCursor :one
SELECT connection_id, kind, value_json
FROM integration_cursors
WHERE connection_id = sqlc.arg(connection_id)
  AND kind = sqlc.arg(kind)
LIMIT 1;

-- name: SaveCursor :exec
INSERT INTO integration_cursors (
  connection_id,
  kind,
  value_json,
  updated_at_ms
) VALUES (
  sqlc.arg(connection_id),
  sqlc.arg(kind),
  sqlc.arg(value_json),
  sqlc.arg(updated_at_ms)
)
ON CONFLICT(connection_id, kind) DO UPDATE SET
  value_json = excluded.value_json,
  updated_at_ms = excluded.updated_at_ms;

-- name: DeleteCursorsForConnection :exec
DELETE FROM integration_cursors
WHERE connection_id = sqlc.arg(connection_id);
