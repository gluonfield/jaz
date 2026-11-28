-- name: InsertAppDraft :exec
INSERT INTO app_drafts (
  id,
  workspace_id,
  target_app_id,
  source_thread_id,
  title,
  description,
  icon_json,
  kind,
  bundle_sha256,
  entry_path,
  storage_dir,
  size_bytes,
  file_count,
  manifest_json,
  capabilities_json,
  allowed_tools_json,
  external_urls_json,
  warnings_json,
  files_json,
  expires_at_ms,
  created_at_ms
) VALUES (
  sqlc.arg(id),
  sqlc.arg(workspace_id),
  sqlc.arg(target_app_id),
  sqlc.arg(source_thread_id),
  sqlc.arg(title),
  sqlc.arg(description),
  sqlc.arg(icon_json),
  sqlc.arg(kind),
  sqlc.arg(bundle_sha256),
  sqlc.arg(entry_path),
  sqlc.arg(storage_dir),
  sqlc.arg(size_bytes),
  sqlc.arg(file_count),
  sqlc.arg(manifest_json),
  sqlc.arg(capabilities_json),
  sqlc.arg(allowed_tools_json),
  sqlc.arg(external_urls_json),
  sqlc.arg(warnings_json),
  sqlc.arg(files_json),
  sqlc.arg(expires_at_ms),
  sqlc.arg(created_at_ms)
);

-- name: GetAppDraft :one
SELECT
  id, workspace_id, target_app_id, source_thread_id, title, description,
  icon_json, kind, bundle_sha256, entry_path, storage_dir, size_bytes,
  file_count, manifest_json, capabilities_json, allowed_tools_json,
  external_urls_json, warnings_json, files_json, expires_at_ms,
  consumed_at_ms, created_at_ms
FROM app_drafts
WHERE id = sqlc.arg(id)
LIMIT 1;

-- name: ConsumeAppDraft :execrows
UPDATE app_drafts
SET consumed_at_ms = sqlc.arg(consumed_at_ms)
WHERE id = sqlc.arg(id)
  AND consumed_at_ms IS NULL
  AND expires_at_ms > sqlc.arg(now_ms)
  AND bundle_sha256 = sqlc.arg(bundle_sha256);

-- name: GetApp :one
SELECT
  a.id, a.workspace_id, a.source_thread_id, a.title, a.description,
  a.icon_json, a.kind, a.visibility, a.current_version, a.status,
  a.created_at_ms, a.updated_at_ms, a.loop_id,
  COALESCE(v.bundle_sha256, '') AS bundle_sha256,
  COALESCE(v.entry_path, '') AS entry_path,
  COALESCE(v.capabilities_json, '[]') AS capabilities_json,
  COALESCE(v.allowed_tools_json, '[]') AS allowed_tools_json
FROM apps a
LEFT JOIN app_versions v ON v.app_id = a.id AND v.version = a.current_version
WHERE a.id = sqlc.arg(id)
  AND a.status = 'active'
LIMIT 1;

-- name: GetAppIDByLoop :one
SELECT id
FROM apps
WHERE loop_id = sqlc.arg(loop_id)
  AND status = 'active'
LIMIT 1;

-- name: GetAppVersionMax :one
SELECT current_version, status
FROM apps
WHERE id = sqlc.arg(id)
LIMIT 1;

-- name: MaxAppVersionRow :one
SELECT CAST(COALESCE(MAX(version), 0) AS INTEGER) AS max_version
FROM app_versions
WHERE app_id = sqlc.arg(app_id);

-- name: ListApps :many
SELECT
  a.id, a.workspace_id, a.source_thread_id, a.title, a.description,
  a.icon_json, a.kind, a.visibility, a.current_version, a.status,
  a.created_at_ms, a.updated_at_ms, a.loop_id,
  COALESCE(v.bundle_sha256, '') AS bundle_sha256,
  COALESCE(v.entry_path, '') AS entry_path,
  COALESCE(v.capabilities_json, '[]') AS capabilities_json,
  COALESCE(v.allowed_tools_json, '[]') AS allowed_tools_json
FROM apps a
LEFT JOIN app_versions v ON v.app_id = a.id AND v.version = a.current_version
WHERE a.status = 'active'
  AND (CAST(sqlc.arg(filter_workspace_id) AS TEXT) = '' OR a.workspace_id = sqlc.arg(filter_workspace_id))
  AND (CAST(sqlc.arg(filter_source_thread_id) AS TEXT) = '' OR a.source_thread_id = sqlc.arg(filter_source_thread_id))
  AND (CAST(sqlc.arg(filter_kind) AS TEXT) = '' OR a.kind = sqlc.arg(filter_kind))
  AND (CAST(sqlc.arg(filter_query) AS TEXT) = '' OR LOWER(a.title) LIKE '%' || LOWER(sqlc.arg(filter_query)) || '%' OR LOWER(a.description) LIKE '%' || LOWER(sqlc.arg(filter_query)) || '%')
ORDER BY a.updated_at_ms DESC, a.id ASC;

-- name: InsertApp :exec
INSERT INTO apps (
  id, workspace_id, source_thread_id, title, description, icon_json,
  kind, visibility, current_version, status, created_at_ms, updated_at_ms, loop_id
) VALUES (
  sqlc.arg(id), sqlc.arg(workspace_id), sqlc.arg(source_thread_id),
  sqlc.arg(title), sqlc.arg(description), sqlc.arg(icon_json),
  sqlc.arg(kind), sqlc.arg(visibility), sqlc.arg(current_version),
  'active', sqlc.arg(created_at_ms), sqlc.arg(updated_at_ms), sqlc.arg(loop_id)
);

-- name: UpdateAppPublished :exec
UPDATE apps
SET title = sqlc.arg(title),
    description = sqlc.arg(description),
    icon_json = sqlc.arg(icon_json),
    kind = sqlc.arg(kind),
    visibility = sqlc.arg(visibility),
    current_version = sqlc.arg(current_version),
    updated_at_ms = sqlc.arg(updated_at_ms)
WHERE id = sqlc.arg(id)
  AND status = 'active';

-- name: UpdateAppMeta :exec
UPDATE apps
SET title = sqlc.arg(title),
    description = sqlc.arg(description),
    icon_json = sqlc.arg(icon_json),
    visibility = sqlc.arg(visibility),
    updated_at_ms = sqlc.arg(updated_at_ms)
WHERE id = sqlc.arg(id)
  AND status = 'active';

-- name: SetAppCurrentVersion :exec
UPDATE apps
SET current_version = sqlc.arg(current_version),
    updated_at_ms = sqlc.arg(updated_at_ms)
WHERE id = sqlc.arg(id)
  AND status = 'active';

-- name: MarkAppDeleting :execrows
UPDATE apps
SET status = 'deleting',
    updated_at_ms = sqlc.arg(updated_at_ms)
WHERE id = sqlc.arg(id)
  AND status = 'active';

-- name: ListAppVersionStorageDirs :many
SELECT storage_dir
FROM app_versions
WHERE app_id = sqlc.arg(app_id);

-- name: DeleteAppDraftsForApp :exec
DELETE FROM app_drafts
WHERE target_app_id = sqlc.arg(app_id);

-- name: DeleteAppRow :exec
DELETE FROM apps
WHERE id = sqlc.arg(id);

-- name: InsertAppVersion :exec
INSERT INTO app_versions (
  app_id, version, bundle_sha256, entry_path, storage_dir,
  size_bytes, file_count, manifest_json, capabilities_json,
  allowed_tools_json, external_urls_json, files_json, created_at_ms
) VALUES (
  sqlc.arg(app_id), sqlc.arg(version), sqlc.arg(bundle_sha256),
  sqlc.arg(entry_path), sqlc.arg(storage_dir), sqlc.arg(size_bytes),
  sqlc.arg(file_count), sqlc.arg(manifest_json), sqlc.arg(capabilities_json),
  sqlc.arg(allowed_tools_json), sqlc.arg(external_urls_json),
  sqlc.arg(files_json), sqlc.arg(created_at_ms)
);

-- name: GetAppVersion :one
SELECT
  app_id, version, bundle_sha256, entry_path, storage_dir,
  size_bytes, file_count, manifest_json, capabilities_json,
  allowed_tools_json, external_urls_json, files_json, created_at_ms
FROM app_versions
WHERE app_id = sqlc.arg(app_id)
  AND version = sqlc.arg(version)
LIMIT 1;

-- name: ListAppVersions :many
SELECT
  app_id, version, bundle_sha256, entry_path, storage_dir,
  size_bytes, file_count, manifest_json, capabilities_json,
  allowed_tools_json, external_urls_json, files_json, created_at_ms
FROM app_versions
WHERE app_id = sqlc.arg(app_id)
ORDER BY version DESC;

-- name: ListAppKV :many
SELECT app_id, scope_type, scope_owner_id, key, value_text, revision, size_bytes, updated_by, updated_at_ms
FROM app_kv
WHERE app_id = sqlc.arg(app_id)
  AND scope_type = sqlc.arg(scope_type)
  AND scope_owner_id = sqlc.arg(scope_owner_id)
  AND (CAST(sqlc.arg(prefix) AS TEXT) = '' OR key LIKE sqlc.arg(prefix) || '%')
ORDER BY key ASC;

-- name: SumAppKVBytes :one
SELECT CAST(COALESCE(SUM(size_bytes), 0) AS INTEGER) AS total_bytes
FROM app_kv
WHERE app_id = sqlc.arg(app_id);

-- name: GetAppKV :one
SELECT app_id, scope_type, scope_owner_id, key, value_text, revision, size_bytes, updated_by, updated_at_ms
FROM app_kv
WHERE app_id = sqlc.arg(app_id)
  AND scope_type = sqlc.arg(scope_type)
  AND scope_owner_id = sqlc.arg(scope_owner_id)
  AND key = sqlc.arg(key)
LIMIT 1;

-- name: UpsertAppKV :exec
INSERT INTO app_kv (
  app_id, scope_type, scope_owner_id, key, value_text, revision, size_bytes, updated_by, updated_at_ms
) VALUES (
  sqlc.arg(app_id), sqlc.arg(scope_type), sqlc.arg(scope_owner_id),
  sqlc.arg(key), sqlc.arg(value_text), sqlc.arg(revision),
  sqlc.arg(size_bytes), sqlc.arg(updated_by), sqlc.arg(updated_at_ms)
)
ON CONFLICT (app_id, scope_type, scope_owner_id, key) DO UPDATE SET
  value_text = excluded.value_text,
  revision = excluded.revision,
  size_bytes = excluded.size_bytes,
  updated_by = excluded.updated_by,
  updated_at_ms = excluded.updated_at_ms;

-- name: DeleteAppKV :exec
DELETE FROM app_kv
WHERE app_id = sqlc.arg(app_id)
  AND scope_type = sqlc.arg(scope_type)
  AND scope_owner_id = sqlc.arg(scope_owner_id)
  AND key = sqlc.arg(key);

-- name: MaxBoardY :one
SELECT CAST(COALESCE(MAX(y + h), 0) AS INTEGER) AS max_y
FROM board_widgets
WHERE board_id = sqlc.arg(board_id);

-- name: UpsertBoardAppWidget :exec
INSERT INTO board_widgets (
  board_id, widget_id, app_id, x, y, w, h, placed_by, created_at_ms, updated_at_ms
) VALUES (
  sqlc.arg(board_id), sqlc.arg(widget_id), sqlc.arg(app_id),
  sqlc.arg(x), sqlc.arg(y), sqlc.arg(w), sqlc.arg(h),
  sqlc.arg(placed_by), sqlc.arg(created_at_ms), sqlc.arg(updated_at_ms)
)
ON CONFLICT (board_id, widget_id) DO UPDATE SET
  app_id = excluded.app_id,
  updated_at_ms = excluded.updated_at_ms;
