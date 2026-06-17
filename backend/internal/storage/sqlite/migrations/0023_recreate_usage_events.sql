-- +goose Up
-- 0019 was edited to add the `source` column after it had already been applied
-- to live databases, so those tables are missing it and every usage read/write
-- fails. usage_events is derived — the usage meter reads thread totals, and the
-- daily/model views are fed by per-turn events going forward — so recreate it
-- with the correct schema. (No backfill: only source='turn' rows are ever
-- aggregated, so importing per-thread snapshots would just be dead rows.)
DROP TABLE IF EXISTS usage_events;

CREATE TABLE usage_events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  thread_id TEXT NOT NULL,
  runtime TEXT NOT NULL,
  agent TEXT NOT NULL DEFAULT '',
  model_provider TEXT NOT NULL DEFAULT '',
  model TEXT NOT NULL DEFAULT '',
  input_tokens INTEGER NOT NULL DEFAULT 0,
  cached_input_tokens INTEGER NOT NULL DEFAULT 0,
  cached_write_tokens INTEGER NOT NULL DEFAULT 0,
  output_tokens INTEGER NOT NULL DEFAULT 0,
  reasoning_output_tokens INTEGER NOT NULL DEFAULT 0,
  total_tokens INTEGER NOT NULL DEFAULT 0,
  context_tokens INTEGER NOT NULL DEFAULT 0,
  context_window_tokens INTEGER NOT NULL DEFAULT 0,
  source TEXT NOT NULL DEFAULT 'turn',
  created_at_ms INTEGER NOT NULL,
  FOREIGN KEY (thread_id) REFERENCES threads(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_usage_events_created_at ON usage_events(created_at_ms);

-- +goose Down
DROP TABLE IF EXISTS usage_events;
