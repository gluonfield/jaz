-- name: GetBoardFontScale :one
SELECT font_scale
FROM board_settings
WHERE board_id = sqlc.arg(board_id);

-- name: UpsertBoardFontScale :exec
INSERT INTO board_settings (board_id, font_scale)
VALUES (sqlc.arg(board_id), sqlc.arg(font_scale))
ON CONFLICT(board_id) DO UPDATE SET font_scale = excluded.font_scale;

-- name: ListBoardTiles :many
SELECT
  t.x, t.y, t.w, t.h, t.placed_by,
  a.id, a.workspace_id, a.source_thread_id, a.title, a.description,
  a.icon_json, a.kind, a.visibility, a.current_version, a.status,
  a.created_at_ms, a.updated_at_ms,
  CAST(COALESCE(la.loop_id, '') AS TEXT) AS loop_id,
  COALESCE(v.bundle_sha256, '') AS bundle_sha256,
  COALESCE(v.entry_path, '') AS entry_path,
  COALESCE(v.capabilities_json, '[]') AS capabilities_json,
  COALESCE(v.allowed_tools_json, '[]') AS allowed_tools_json,
  CAST(COALESCE(la.last_error, '') AS TEXT) AS last_error,
  CAST(COALESCE(l.name, '') AS TEXT) AS loop_name,
  CAST(COALESCE(l.status, '') AS TEXT) AS loop_status,
  CAST(COALESCE(l.last_run_status, '') AS TEXT) AS loop_last_run_status,
  CAST(COALESCE(l.last_run_at_ms, 0) AS INTEGER) AS loop_last_run_at_ms
FROM board_tiles t
JOIN apps a ON a.id = t.app_id AND a.status = 'active'
LEFT JOIN app_versions v ON v.app_id = a.id AND v.version = a.current_version
LEFT JOIN loop_apps la ON la.app_id = a.id
LEFT JOIN loops l ON l.id = la.loop_id
WHERE t.board_id = sqlc.arg(board_id)
ORDER BY t.y ASC, t.x ASC;

-- name: UpsertBoardTile :exec
INSERT INTO board_tiles (
  board_id, app_id, x, y, w, h, placed_by, created_at_ms, updated_at_ms
) VALUES (
  sqlc.arg(board_id), sqlc.arg(app_id),
  sqlc.arg(x), sqlc.arg(y), sqlc.arg(w), sqlc.arg(h),
  sqlc.arg(placed_by), sqlc.arg(now_ms), sqlc.arg(now_ms)
)
ON CONFLICT(board_id, app_id) DO UPDATE SET
  x = excluded.x,
  y = excluded.y,
  w = excluded.w,
  h = excluded.h,
  placed_by = excluded.placed_by,
  updated_at_ms = excluded.updated_at_ms;

-- name: DeleteBoardTile :exec
DELETE FROM board_tiles
WHERE board_id = sqlc.arg(board_id) AND app_id = sqlc.arg(app_id);

-- name: ListAppBoards :many
SELECT board_id
FROM board_tiles
WHERE app_id = sqlc.arg(app_id);

-- name: ResizeLLMTiles :exec
UPDATE board_tiles
SET w = sqlc.arg(w),
    h = sqlc.arg(h),
    updated_at_ms = sqlc.arg(now_ms)
WHERE app_id = sqlc.arg(app_id)
  AND placed_by = 'llm';

-- name: InsertLoopApp :exec
INSERT INTO loop_apps (loop_id, app_id)
VALUES (sqlc.arg(loop_id), sqlc.arg(app_id));

-- name: GetLoopApp :one
SELECT la.loop_id, la.app_id, la.size_hint, la.last_error, la.last_layout, la.published_run_id
FROM loop_apps la
JOIN apps a ON a.id = la.app_id AND a.status = 'active'
WHERE la.loop_id = sqlc.arg(loop_id);

-- name: UpdateLoopApp :exec
UPDATE loop_apps
SET size_hint = sqlc.arg(size_hint),
    last_error = sqlc.arg(last_error),
    last_layout = sqlc.arg(last_layout),
    published_run_id = sqlc.arg(published_run_id)
WHERE loop_id = sqlc.arg(loop_id);

-- name: SetLoopAppError :exec
UPDATE loop_apps
SET last_error = sqlc.arg(last_error)
WHERE app_id = sqlc.arg(app_id);

-- name: SetLoopAppLayout :exec
UPDATE loop_apps
SET last_layout = sqlc.arg(last_layout)
WHERE app_id = sqlc.arg(app_id);

-- name: DeleteOrphanLoopAppTiles :exec
DELETE FROM board_tiles
WHERE app_id IN (
  SELECT la.app_id FROM loop_apps la
  LEFT JOIN loops l ON l.id = la.loop_id
  WHERE l.id IS NULL OR l.status = 'deleted');

-- name: DeleteUnpublishedOrphanLoopApps :exec
DELETE FROM apps
WHERE current_version = 0
  AND id IN (
  SELECT la.app_id FROM loop_apps la
  LEFT JOIN loops l ON l.id = la.loop_id
  WHERE l.id IS NULL OR l.status = 'deleted');

-- name: DeleteOrphanLoopApps :execrows
DELETE FROM loop_apps
WHERE loop_id NOT IN (SELECT l.id FROM loops l WHERE l.status <> 'deleted');

-- name: ListLegacyWidgetHTML :many
SELECT h.app_id, a.title, h.html
FROM legacy_widget_html h
JOIN apps a ON a.id = h.app_id AND a.status = 'active';

-- name: DeleteLegacyWidgetHTML :exec
DELETE FROM legacy_widget_html
WHERE app_id = sqlc.arg(app_id);
