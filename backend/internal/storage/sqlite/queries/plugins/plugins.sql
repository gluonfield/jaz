-- name: ListPluginSources :many
SELECT id, repository, url, ref, paths, commit_sha, auto_update, is_marketplace, catalog_json, checked_at_ms, updated_at_ms
FROM plugin_sources
ORDER BY id ASC;

-- name: GetPluginSource :one
SELECT id, repository, url, ref, paths, commit_sha, auto_update, is_marketplace, catalog_json, checked_at_ms, updated_at_ms
FROM plugin_sources
WHERE id = sqlc.arg(id)
LIMIT 1;

-- name: UpsertPluginSource :exec
INSERT INTO plugin_sources (id, repository, url, ref, paths, commit_sha, auto_update, is_marketplace, catalog_json, checked_at_ms, updated_at_ms)
VALUES (
  sqlc.arg(id),
  sqlc.arg(repository),
  sqlc.arg(url),
  sqlc.arg(ref),
  sqlc.arg(paths),
  sqlc.arg(commit_sha),
  sqlc.arg(auto_update),
  sqlc.arg(is_marketplace),
  sqlc.arg(catalog_json),
  sqlc.arg(checked_at_ms),
  sqlc.arg(updated_at_ms)
)
ON CONFLICT(id) DO UPDATE SET
  repository = excluded.repository,
  url = excluded.url,
  ref = excluded.ref,
  paths = excluded.paths,
  commit_sha = excluded.commit_sha,
  auto_update = excluded.auto_update,
  is_marketplace = excluded.is_marketplace,
  catalog_json = excluded.catalog_json,
  checked_at_ms = excluded.checked_at_ms,
  updated_at_ms = excluded.updated_at_ms;

-- name: DeletePluginSource :execrows
DELETE FROM plugin_sources
WHERE id = sqlc.arg(id);

-- name: ListPluginStates :many
SELECT plugin_id, enabled, source_id, source_repository, source_ref, source_path, commit_sha, remote_sha, config_json, updated_at_ms
FROM plugin_state
ORDER BY plugin_id ASC;

-- name: GetPluginState :one
SELECT plugin_id, enabled, source_id, source_repository, source_ref, source_path, commit_sha, remote_sha, config_json, updated_at_ms
FROM plugin_state
WHERE plugin_id = sqlc.arg(plugin_id)
LIMIT 1;

-- name: UpsertPluginState :exec
INSERT INTO plugin_state (plugin_id, enabled, source_id, source_repository, source_ref, source_path, commit_sha, remote_sha, config_json, updated_at_ms)
VALUES (
  sqlc.arg(plugin_id),
  sqlc.arg(enabled),
  sqlc.arg(source_id),
  sqlc.arg(source_repository),
  sqlc.arg(source_ref),
  sqlc.arg(source_path),
  sqlc.arg(commit_sha),
  sqlc.arg(remote_sha),
  sqlc.arg(config_json),
  sqlc.arg(updated_at_ms)
)
ON CONFLICT(plugin_id) DO UPDATE SET
  enabled = excluded.enabled,
  source_id = excluded.source_id,
  source_repository = excluded.source_repository,
  source_ref = excluded.source_ref,
  source_path = excluded.source_path,
  commit_sha = excluded.commit_sha,
  remote_sha = excluded.remote_sha,
  config_json = excluded.config_json,
  updated_at_ms = excluded.updated_at_ms;

-- name: DeletePluginState :exec
DELETE FROM plugin_state
WHERE plugin_id = sqlc.arg(plugin_id);

-- name: GetThreadPluginState :one
SELECT thread_id, plugin_id, view, state_key, data_json, ui_state_json, revision, updated_at_ms
FROM thread_plugin_state
WHERE thread_id = sqlc.arg(thread_id)
  AND plugin_id = sqlc.arg(plugin_id)
  AND view = sqlc.arg(view)
  AND state_key = sqlc.arg(state_key)
LIMIT 1;

-- name: ListThreadPluginStatesByPlugin :many
SELECT
  s.thread_id,
  s.plugin_id,
  s.view,
  s.state_key,
  s.data_json,
  s.ui_state_json,
  s.revision,
  s.updated_at_ms,
  t.title AS thread_title,
  t.status AS thread_status,
  t.updated_at_ms AS thread_updated_at_ms,
  t.cwd AS thread_cwd
FROM thread_plugin_state s
JOIN threads t ON t.id = s.thread_id
WHERE s.plugin_id = sqlc.arg(plugin_id)
ORDER BY t.updated_at_ms DESC, s.thread_id ASC;

-- name: UpsertThreadPluginData :exec
INSERT INTO thread_plugin_state (thread_id, plugin_id, view, state_key, data_json, ui_state_json, revision, updated_at_ms)
VALUES (
  sqlc.arg(thread_id),
  sqlc.arg(plugin_id),
  sqlc.arg(view),
  sqlc.arg(state_key),
  sqlc.arg(data_json),
  'null',
  1,
  sqlc.arg(updated_at_ms)
)
ON CONFLICT(thread_id, plugin_id, view, state_key) DO UPDATE SET
  data_json = excluded.data_json,
  revision = thread_plugin_state.revision + 1,
  updated_at_ms = excluded.updated_at_ms;

-- name: UpsertThreadPluginUIState :exec
INSERT INTO thread_plugin_state (thread_id, plugin_id, view, state_key, data_json, ui_state_json, revision, updated_at_ms)
VALUES (
  sqlc.arg(thread_id),
  sqlc.arg(plugin_id),
  sqlc.arg(view),
  sqlc.arg(state_key),
  'null',
  sqlc.arg(ui_state_json),
  1,
  sqlc.arg(updated_at_ms)
)
ON CONFLICT(thread_id, plugin_id, view, state_key) DO UPDATE SET
  ui_state_json = excluded.ui_state_json,
  revision = thread_plugin_state.revision + 1,
  updated_at_ms = excluded.updated_at_ms;

-- name: UpdateThreadPluginUIStateIfMatch :execrows
UPDATE thread_plugin_state
SET ui_state_json = sqlc.arg(ui_state_json),
    revision = revision + 1,
    updated_at_ms = sqlc.arg(updated_at_ms)
WHERE thread_id = sqlc.arg(thread_id)
  AND plugin_id = sqlc.arg(plugin_id)
  AND view = sqlc.arg(view)
  AND state_key = sqlc.arg(state_key)
  AND revision = sqlc.arg(expected_revision);
