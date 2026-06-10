-- +goose Up
ALTER TABLE loops ADD COLUMN widget_enabled INTEGER NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS widgets (
  id TEXT PRIMARY KEY,
  loop_id TEXT NOT NULL UNIQUE,
  title TEXT NOT NULL DEFAULT '',
  current_version INTEGER NOT NULL DEFAULT 0,
  size_hint TEXT NOT NULL DEFAULT '',
  last_error TEXT,
  created_at_ms INTEGER NOT NULL,
  updated_at_ms INTEGER NOT NULL,
  FOREIGN KEY (loop_id) REFERENCES loops(id)
);

CREATE TABLE IF NOT EXISTS widget_versions (
  widget_id TEXT NOT NULL,
  version INTEGER NOT NULL,
  html TEXT NOT NULL,
  produced_by_run_id TEXT,
  created_at_ms INTEGER NOT NULL,
  PRIMARY KEY (widget_id, version),
  FOREIGN KEY (widget_id) REFERENCES widgets(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS boards (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  grid_cols INTEGER NOT NULL DEFAULT 6,
  row_height INTEGER NOT NULL DEFAULT 120,
  window_bounds TEXT,
  is_default INTEGER NOT NULL DEFAULT 0,
  created_at_ms INTEGER NOT NULL,
  updated_at_ms INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS board_widgets (
  board_id TEXT NOT NULL,
  widget_id TEXT NOT NULL,
  x INTEGER NOT NULL DEFAULT 0,
  y INTEGER NOT NULL DEFAULT 0,
  w INTEGER NOT NULL DEFAULT 2,
  h INTEGER NOT NULL DEFAULT 2,
  placed_by TEXT NOT NULL DEFAULT 'llm',
  created_at_ms INTEGER NOT NULL,
  updated_at_ms INTEGER NOT NULL,
  PRIMARY KEY (board_id, widget_id),
  FOREIGN KEY (board_id) REFERENCES boards(id) ON DELETE CASCADE,
  FOREIGN KEY (widget_id) REFERENCES widgets(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_widgets_loop ON widgets(loop_id);
CREATE INDEX IF NOT EXISTS idx_widget_versions_widget ON widget_versions(widget_id, version DESC);
CREATE INDEX IF NOT EXISTS idx_board_widgets_widget ON board_widgets(widget_id);

-- +goose Down
DROP TABLE IF EXISTS board_widgets;
DROP TABLE IF EXISTS boards;
DROP TABLE IF EXISTS widget_versions;
DROP TABLE IF EXISTS widgets;
ALTER TABLE loops DROP COLUMN widget_enabled;
