-- name: ListMCPServers :many
SELECT
  id,
  name,
  transport,
  url,
  enabled,
  bearer_token_env_var,
  headers_json,
  env_headers_json,
  created_at_ms,
  updated_at_ms
FROM mcp_servers
ORDER BY updated_at_ms DESC;

-- name: GetMCPServer :one
SELECT
  id,
  name,
  transport,
  url,
  enabled,
  bearer_token_env_var,
  headers_json,
  env_headers_json,
  created_at_ms,
  updated_at_ms
FROM mcp_servers
WHERE id = sqlc.arg(id)
LIMIT 1;

-- name: CreateMCPServer :exec
INSERT INTO mcp_servers (
  id,
  name,
  transport,
  url,
  enabled,
  bearer_token_env_var,
  headers_json,
  env_headers_json,
  created_at_ms,
  updated_at_ms
) VALUES (
  sqlc.arg(id),
  sqlc.arg(name),
  sqlc.arg(transport),
  sqlc.arg(url),
  sqlc.arg(enabled),
  sqlc.narg(bearer_token_env_var),
  sqlc.arg(headers_json),
  sqlc.arg(env_headers_json),
  sqlc.arg(created_at_ms),
  sqlc.arg(updated_at_ms)
);

-- name: UpdateMCPServer :execrows
UPDATE mcp_servers
SET
  name = sqlc.arg(name),
  transport = sqlc.arg(transport),
  url = sqlc.arg(url),
  enabled = sqlc.arg(enabled),
  bearer_token_env_var = sqlc.narg(bearer_token_env_var),
  headers_json = sqlc.arg(headers_json),
  env_headers_json = sqlc.arg(env_headers_json),
  updated_at_ms = sqlc.arg(updated_at_ms)
WHERE id = sqlc.arg(id);

-- name: DeleteMCPServer :execrows
DELETE FROM mcp_servers
WHERE id = sqlc.arg(id);

-- name: SetMCPServerEnabled :execrows
UPDATE mcp_servers
SET
  enabled = sqlc.arg(enabled),
  updated_at_ms = sqlc.arg(updated_at_ms)
WHERE id = sqlc.arg(id);

-- name: GetMCPOAuthToken :one
SELECT token_json
FROM mcp_oauth_tokens
WHERE server_id = sqlc.arg(server_id)
LIMIT 1;

-- name: UpsertMCPOAuthToken :exec
INSERT INTO mcp_oauth_tokens (
  server_id,
  token_json,
  updated_at_ms
) VALUES (
  sqlc.arg(server_id),
  sqlc.arg(token_json),
  sqlc.arg(updated_at_ms)
)
ON CONFLICT(server_id) DO UPDATE SET
  token_json = excluded.token_json,
  updated_at_ms = excluded.updated_at_ms;

-- name: DeleteMCPOAuthToken :exec
DELETE FROM mcp_oauth_tokens
WHERE server_id = sqlc.arg(server_id);
