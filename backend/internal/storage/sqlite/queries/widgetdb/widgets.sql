-- name: UpsertWidget :exec
INSERT INTO widgets (
  id,
  loop_id,
  title,
  size_hint,
  last_error,
  last_layout,
  created_at_ms,
  updated_at_ms,
  workspace_id,
  published_run_id
) VALUES (
  sqlc.arg(id),
  sqlc.arg(loop_id),
  sqlc.arg(title),
  sqlc.arg(size_hint),
  sqlc.narg(last_error),
  sqlc.arg(last_layout),
  sqlc.arg(created_at_ms),
  sqlc.arg(updated_at_ms),
  sqlc.arg(workspace_id),
  sqlc.arg(published_run_id)
)
ON CONFLICT(id) DO UPDATE SET
  loop_id = excluded.loop_id,
  title = excluded.title,
  size_hint = excluded.size_hint,
  last_error = excluded.last_error,
  last_layout = excluded.last_layout,
  created_at_ms = excluded.created_at_ms,
  updated_at_ms = excluded.updated_at_ms,
  workspace_id = excluded.workspace_id,
  published_run_id = excluded.published_run_id;

-- name: GetWidget :one
SELECT id, loop_id, title, current_version, size_hint, last_error, created_at_ms, updated_at_ms, last_layout, workspace_id, published_run_id
FROM widgets
WHERE id = sqlc.arg(id)
LIMIT 1;

-- name: GetWidgetByLoop :one
SELECT id, loop_id, title, current_version, size_hint, last_error, created_at_ms, updated_at_ms, last_layout, workspace_id, published_run_id
FROM widgets
WHERE loop_id = sqlc.arg(loop_id)
LIMIT 1;

-- name: ListLegacyWidgetHTML :many
SELECT w.id, w.loop_id, CAST(COALESCE(NULLIF(w.title, ''), l.name) AS TEXT) AS title, v.html
FROM widgets w
JOIN loops l ON l.id = w.loop_id AND l.status <> sqlc.arg(deleted_status)
JOIN widget_versions v ON v.widget_id = w.id AND v.version = w.current_version;

-- name: DeleteWidgetVersions :exec
DELETE FROM widget_versions
WHERE widget_id = sqlc.arg(widget_id);

-- name: UpsertBoard :exec
INSERT INTO boards (
  id,
  name,
  grid_cols,
  row_height,
  font_scale,
  window_bounds,
  is_default,
  created_at_ms,
  updated_at_ms,
  workspace_id
) VALUES (
  sqlc.arg(id),
  sqlc.arg(name),
  sqlc.arg(grid_cols),
  sqlc.arg(row_height),
  sqlc.arg(font_scale),
  sqlc.narg(window_bounds),
  sqlc.arg(is_default),
  sqlc.arg(created_at_ms),
  sqlc.arg(updated_at_ms),
  sqlc.arg(workspace_id)
)
ON CONFLICT(id) DO UPDATE SET
  name = excluded.name,
  grid_cols = excluded.grid_cols,
  row_height = excluded.row_height,
  font_scale = excluded.font_scale,
  window_bounds = excluded.window_bounds,
  is_default = excluded.is_default,
  created_at_ms = excluded.created_at_ms,
  updated_at_ms = excluded.updated_at_ms,
  workspace_id = excluded.workspace_id;

-- name: GetBoard :one
SELECT id, name, grid_cols, row_height, window_bounds, is_default, created_at_ms, updated_at_ms, font_scale, workspace_id
FROM boards
WHERE id = sqlc.arg(id)
LIMIT 1;

-- name: GetDefaultBoard :one
SELECT id, name, grid_cols, row_height, window_bounds, is_default, created_at_ms, updated_at_ms, font_scale, workspace_id
FROM boards
WHERE is_default = 1
  AND workspace_id = sqlc.arg(workspace_id)
ORDER BY created_at_ms ASC
LIMIT 1;

-- name: ListBoards :many
SELECT id, name, grid_cols, row_height, window_bounds, is_default, created_at_ms, updated_at_ms, font_scale, workspace_id
FROM boards
WHERE (CAST(sqlc.arg(filter_workspace_id) AS TEXT) = '' OR workspace_id = sqlc.arg(filter_workspace_id))
ORDER BY created_at_ms ASC;

-- name: DeleteBoard :exec
DELETE FROM boards
WHERE id = sqlc.arg(id);

-- name: UpsertBoardWidget :exec
INSERT INTO board_widgets (
  board_id,
  widget_id,
  x,
  y,
  w,
  h,
  placed_by,
  created_at_ms,
  updated_at_ms
) VALUES (
  sqlc.arg(board_id),
  sqlc.arg(widget_id),
  sqlc.arg(x),
  sqlc.arg(y),
  sqlc.arg(w),
  sqlc.arg(h),
  sqlc.arg(placed_by),
  sqlc.arg(created_at_ms),
  sqlc.arg(updated_at_ms)
)
ON CONFLICT(board_id, widget_id) DO UPDATE SET
  x = excluded.x,
  y = excluded.y,
  w = excluded.w,
  h = excluded.h,
  placed_by = excluded.placed_by,
  updated_at_ms = excluded.updated_at_ms;

-- name: GetBoardWidget :one
SELECT board_id, widget_id, x, y, w, h, placed_by, created_at_ms, updated_at_ms
FROM board_widgets
WHERE board_id = sqlc.arg(board_id) AND widget_id = sqlc.arg(widget_id)
LIMIT 1;

-- name: DeleteBoardWidget :exec
DELETE FROM board_widgets
WHERE board_id = sqlc.arg(board_id) AND widget_id = sqlc.arg(widget_id);

-- name: ListBoardsForWidget :many
SELECT board_id
FROM board_widgets
WHERE widget_id = sqlc.arg(widget_id);

-- name: ListBoardItems :many
SELECT
  bw.board_id,
  bw.widget_id,
  CAST(COALESCE(a.id, '') AS TEXT) AS app_id,
  CAST(COALESCE(a.kind, '') AS TEXT) AS app_kind,
  bw.x,
  bw.y,
  bw.w,
  bw.h,
  bw.placed_by,
  CAST(COALESCE(l.id, '') AS TEXT) AS loop_id,
  CAST(COALESCE(a.title, w.title, '') AS TEXT) AS title,
  CAST(COALESCE(a.current_version, 0) AS INTEGER) AS current_version,
  CAST(COALESCE(w.size_hint, '') AS TEXT) AS size_hint,
  CAST(COALESCE(w.last_error, '') AS TEXT) AS last_error,
  CAST(COALESCE(a.updated_at_ms, w.updated_at_ms, 0) AS INTEGER) AS updated_at_ms,
  CAST(COALESCE(l.name, '') AS TEXT) AS loop_name,
  CAST(COALESCE(l.status, '') AS TEXT) AS loop_status,
  CAST(COALESCE(l.last_run_status, '') AS TEXT) AS loop_last_run_status,
  CAST(COALESCE(l.last_run_at_ms, 0) AS INTEGER) AS loop_last_run_at_ms
FROM board_widgets bw
LEFT JOIN widgets w ON bw.app_id IS NULL AND w.id = bw.widget_id
LEFT JOIN loops l ON l.id = w.loop_id AND l.status <> sqlc.arg(deleted_status)
LEFT JOIN apps a ON a.status = 'active' AND (a.id = bw.app_id OR a.loop_id = l.id)
WHERE bw.board_id = sqlc.arg(board_id)
  AND (l.id IS NOT NULL OR a.id IS NOT NULL)
ORDER BY bw.y ASC, bw.x ASC;

-- name: ListAllPlacements :many
SELECT board_id, widget_id, x, y, w, h, placed_by, created_at_ms, updated_at_ms
FROM board_widgets
WHERE board_id = sqlc.arg(board_id);

-- name: DeleteOrphanBoardWidgets :exec
DELETE FROM board_widgets
WHERE app_id IS NULL
  AND widget_id IN (
  SELECT w.id FROM widgets w
  LEFT JOIN loops l ON l.id = w.loop_id
  WHERE l.id IS NULL OR l.status = sqlc.arg(deleted_status));

-- name: DeleteOrphanWidgetVersions :exec
DELETE FROM widget_versions WHERE widget_id IN (
  SELECT w.id FROM widgets w
  LEFT JOIN loops l ON l.id = w.loop_id
  WHERE l.id IS NULL OR l.status = sqlc.arg(deleted_status));

-- name: DeleteOrphanWidgets :execrows
DELETE FROM widgets WHERE id IN (
  SELECT w.id FROM widgets w
  LEFT JOIN loops l ON l.id = w.loop_id
  WHERE l.id IS NULL OR l.status = sqlc.arg(deleted_status));

-- name: ReleaseOrphanLoopApps :exec
UPDATE apps SET loop_id = ''
WHERE loop_id <> ''
  AND loop_id NOT IN (SELECT l.id FROM loops l WHERE l.status <> sqlc.arg(deleted_status));
