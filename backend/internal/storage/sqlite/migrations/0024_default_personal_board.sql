-- +goose Up
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
)
SELECT
  'board-personal',
  'Personal',
  6,
  120,
  1,
  NULL,
  1,
  CAST(strftime('%s', 'now') AS INTEGER) * 1000,
  CAST(strftime('%s', 'now') AS INTEGER) * 1000
WHERE NOT EXISTS (SELECT 1 FROM boards);

-- +goose Down
DELETE FROM boards
WHERE id = 'board-personal'
  AND is_default = 1
  AND NOT EXISTS (
    SELECT 1 FROM board_widgets WHERE board_id = 'board-personal'
  );
