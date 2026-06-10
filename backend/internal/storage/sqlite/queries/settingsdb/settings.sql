-- name: GetSetting :one
SELECT
  namespace,
  key,
  value_json,
  created_at_ms,
  updated_at_ms
FROM settings
WHERE namespace = sqlc.arg(namespace)
  AND key = sqlc.arg(key)
LIMIT 1;

-- name: UpsertSetting :exec
INSERT INTO settings (
  namespace,
  key,
  value_json,
  created_at_ms,
  updated_at_ms
) VALUES (
  sqlc.arg(namespace),
  sqlc.arg(key),
  sqlc.arg(value_json),
  sqlc.arg(created_at_ms),
  sqlc.arg(updated_at_ms)
)
ON CONFLICT(namespace, key) DO UPDATE SET
  value_json = excluded.value_json,
  updated_at_ms = excluded.updated_at_ms;

-- name: DeleteSetting :execrows
DELETE FROM settings
WHERE namespace = sqlc.arg(namespace)
  AND key = sqlc.arg(key);

-- name: ListSettings :many
SELECT
  namespace,
  key,
  value_json,
  created_at_ms,
  updated_at_ms
FROM settings
WHERE namespace = sqlc.arg(namespace)
ORDER BY key;
