-- name: UpsertWidget :exec
INSERT INTO widgets (
  id,
  loop_id,
  title,
  current_version,
  size_hint,
  last_error,
  last_layout,
  created_at_ms,
  updated_at_ms
) VALUES (
  sqlc.arg(id),
  sqlc.arg(loop_id),
  sqlc.arg(title),
  sqlc.arg(current_version),
  sqlc.arg(size_hint),
  sqlc.narg(last_error),
  sqlc.arg(last_layout),
  sqlc.arg(created_at_ms),
  sqlc.arg(updated_at_ms)
)
ON CONFLICT(id) DO UPDATE SET
  loop_id = excluded.loop_id,
  title = excluded.title,
  current_version = excluded.current_version,
  size_hint = excluded.size_hint,
  last_error = excluded.last_error,
  last_layout = excluded.last_layout,
  created_at_ms = excluded.created_at_ms,
  updated_at_ms = excluded.updated_at_ms;

-- name: GetWidget :one
SELECT id, loop_id, title, current_version, size_hint, last_error, created_at_ms, updated_at_ms, last_layout
FROM widgets
WHERE id = sqlc.arg(id)
LIMIT 1;

-- name: GetWidgetByLoop :one
SELECT id, loop_id, title, current_version, size_hint, last_error, created_at_ms, updated_at_ms, last_layout
FROM widgets
WHERE loop_id = sqlc.arg(loop_id)
LIMIT 1;

-- name: ListWidgets :many
SELECT id, loop_id, title, current_version, size_hint, last_error, created_at_ms, updated_at_ms, last_layout
FROM widgets
ORDER BY updated_at_ms DESC;

-- name: InsertWidgetVersion :exec
INSERT INTO widget_versions (
  widget_id,
  version,
  html,
  produced_by_run_id,
  created_at_ms
) VALUES (
  sqlc.arg(widget_id),
  sqlc.arg(version),
  sqlc.arg(html),
  sqlc.narg(produced_by_run_id),
  sqlc.arg(created_at_ms)
);

-- name: GetWidgetVersion :one
SELECT widget_id, version, html, produced_by_run_id, created_at_ms
FROM widget_versions
WHERE widget_id = sqlc.arg(widget_id) AND version = sqlc.arg(version)
LIMIT 1;

-- name: PruneWidgetVersions :exec
DELETE FROM widget_versions
WHERE widget_id = sqlc.arg(widget_id)
  AND version <= sqlc.arg(max_keep_version);

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
  updated_at_ms
) VALUES (
  sqlc.arg(id),
  sqlc.arg(name),
  sqlc.arg(grid_cols),
  sqlc.arg(row_height),
  sqlc.arg(font_scale),
  sqlc.narg(window_bounds),
  sqlc.arg(is_default),
  sqlc.arg(created_at_ms),
  sqlc.arg(updated_at_ms)
)
ON CONFLICT(id) DO UPDATE SET
  name = excluded.name,
  grid_cols = excluded.grid_cols,
  row_height = excluded.row_height,
  font_scale = excluded.font_scale,
  window_bounds = excluded.window_bounds,
  is_default = excluded.is_default,
  created_at_ms = excluded.created_at_ms,
  updated_at_ms = excluded.updated_at_ms;

-- name: GetBoard :one
SELECT id, name, grid_cols, row_height, window_bounds, is_default, created_at_ms, updated_at_ms, font_scale
FROM boards
WHERE id = sqlc.arg(id)
LIMIT 1;

-- name: GetDefaultBoard :one
SELECT id, name, grid_cols, row_height, window_bounds, is_default, created_at_ms, updated_at_ms, font_scale
FROM boards
WHERE is_default = 1
ORDER BY created_at_ms ASC
LIMIT 1;

-- name: ListBoards :many
SELECT id, name, grid_cols, row_height, window_bounds, is_default, created_at_ms, updated_at_ms, font_scale
FROM boards
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
  bw.x,
  bw.y,
  bw.w,
  bw.h,
  bw.placed_by,
  w.loop_id,
  w.title,
  w.current_version,
  w.size_hint,
  w.last_error,
  w.updated_at_ms AS widget_updated_at_ms,
  l.name AS loop_name,
  l.status AS loop_status,
  l.last_run_status AS loop_last_run_status,
  l.last_run_at_ms AS loop_last_run_at_ms
FROM board_widgets bw
JOIN widgets w ON w.id = bw.widget_id
JOIN loops l ON l.id = w.loop_id
WHERE bw.board_id = sqlc.arg(board_id)
  AND l.status <> sqlc.arg(deleted_status)
ORDER BY bw.y ASC, bw.x ASC;

-- name: ListAllPlacements :many
SELECT board_id, widget_id, x, y, w, h, placed_by, created_at_ms, updated_at_ms
FROM board_widgets
WHERE board_id = sqlc.arg(board_id);

-- name: DeleteOrphanBoardWidgets :exec
DELETE FROM board_widgets WHERE widget_id IN (
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
