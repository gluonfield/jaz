-- name: LoadConnection :one
SELECT
  c.id,
  c.provider,
  c.account_id,
  c.account_name,
  c.alias,
  c.scopes_json,
  CAST(COALESCE(MAX(ic.updated_at_ms), 0) AS INTEGER) AS last_synced_at_ms
FROM integration_connections c
LEFT JOIN integration_cursors ic ON ic.connection_id = c.id
WHERE c.id = sqlc.arg(id)
GROUP BY c.id, c.provider, c.account_id, c.account_name, c.alias, c.scopes_json
LIMIT 1;

-- name: ListConnectionsByProvider :many
SELECT
  c.id,
  c.provider,
  c.account_id,
  c.account_name,
  c.alias,
  c.scopes_json,
  CAST(COALESCE(MAX(ic.updated_at_ms), 0) AS INTEGER) AS last_synced_at_ms
FROM integration_connections c
LEFT JOIN integration_cursors ic ON ic.connection_id = c.id
WHERE c.provider = sqlc.arg(provider)
GROUP BY c.id, c.provider, c.account_id, c.account_name, c.alias, c.scopes_json
ORDER BY c.alias, c.account_id, c.id;

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
