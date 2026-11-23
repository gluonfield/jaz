-- name: GetLocalInstruction :one
SELECT scope, scope_id, kind, body_md, version, updated_at_ms
FROM local_instructions
WHERE scope = sqlc.arg(scope)
  AND scope_id = sqlc.arg(scope_id)
  AND kind = sqlc.arg(kind);

-- name: UpsertLocalInstruction :exec
INSERT INTO local_instructions (scope, scope_id, kind, body_md, version, updated_at_ms)
VALUES (
  sqlc.arg(scope),
  sqlc.arg(scope_id),
  sqlc.arg(kind),
  sqlc.arg(body_md),
  sqlc.arg(version),
  sqlc.arg(updated_at_ms)
)
ON CONFLICT(scope, scope_id, kind) DO UPDATE SET
  body_md = excluded.body_md,
  version = excluded.version,
  updated_at_ms = excluded.updated_at_ms;
