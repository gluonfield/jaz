-- name: LoadToken :one
SELECT token_json
FROM integration_oauth_tokens
WHERE connection_id = sqlc.arg(connection_id)
LIMIT 1;

-- name: SaveToken :exec
INSERT INTO integration_oauth_tokens (
  connection_id,
  token_json,
  updated_at_ms
) VALUES (
  sqlc.arg(connection_id),
  sqlc.arg(token_json),
  sqlc.arg(updated_at_ms)
)
ON CONFLICT(connection_id) DO UPDATE SET
  token_json = excluded.token_json,
  updated_at_ms = excluded.updated_at_ms;

-- name: DeleteToken :exec
DELETE FROM integration_oauth_tokens
WHERE connection_id = sqlc.arg(connection_id);
