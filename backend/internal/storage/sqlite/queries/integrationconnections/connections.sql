-- name: LoadConnection :one
SELECT id, provider, account_id, account_name, alias, scopes_json
FROM integration_connections
WHERE id = sqlc.arg(id)
LIMIT 1;

-- name: ListConnectionsByProvider :many
SELECT id, provider, account_id, account_name, alias, scopes_json
FROM integration_connections
WHERE provider = sqlc.arg(provider)
ORDER BY alias, account_id, id;

-- name: SaveConnection :exec
INSERT INTO integration_connections (
  id,
  provider,
  account_id,
  account_name,
  alias,
  scopes_json,
  updated_at_ms
) VALUES (
  sqlc.arg(id),
  sqlc.arg(provider),
  sqlc.arg(account_id),
  sqlc.arg(account_name),
  sqlc.arg(alias),
  sqlc.arg(scopes_json),
  sqlc.arg(updated_at_ms)
)
ON CONFLICT(id) DO UPDATE SET
  provider = excluded.provider,
  account_id = excluded.account_id,
  account_name = excluded.account_name,
  alias = excluded.alias,
  scopes_json = excluded.scopes_json,
  updated_at_ms = excluded.updated_at_ms;

-- name: DeleteConnection :execrows
DELETE FROM integration_connections
WHERE id = sqlc.arg(id);
