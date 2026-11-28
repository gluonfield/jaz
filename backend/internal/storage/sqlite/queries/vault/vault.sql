-- name: ListVaultItems :many
SELECT id, kind, label, origins_json, meta_json, created_at_ms, updated_at_ms
FROM vault_items
WHERE (@filter_kind = '' OR kind = @filter_kind)
ORDER BY updated_at_ms DESC, id ASC;

-- name: ListVaultItemSecrets :many
SELECT id, secret_encrypted
FROM vault_items
ORDER BY id ASC;

-- name: GetVaultItemMeta :one
SELECT id, kind, label, origins_json, meta_json, created_at_ms, updated_at_ms
FROM vault_items
WHERE id = ?;

-- name: GetVaultItemRecord :one
SELECT id, kind, label, origins_json, meta_json, secret_encrypted, created_at_ms, updated_at_ms
FROM vault_items
WHERE id = ?;

-- name: InsertVaultItem :exec
INSERT INTO vault_items (
  id, kind, label, origins_json, meta_json, secret_encrypted, created_at_ms, updated_at_ms
) VALUES (?, ?, ?, ?, ?, ?, ?, ?);

-- name: UpdateVaultItem :execrows
UPDATE vault_items
SET label = ?,
    origins_json = ?,
    meta_json = ?,
    secret_encrypted = ?,
    updated_at_ms = ?
WHERE id = ?;

-- name: UpdateVaultItemSecret :exec
UPDATE vault_items
SET secret_encrypted = ?
WHERE id = ?;

-- name: DeleteVaultItem :execrows
DELETE FROM vault_items
WHERE id = ?;
