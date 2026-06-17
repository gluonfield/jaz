-- +goose Up
CREATE TABLE IF NOT EXISTS usage_events (
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

INSERT INTO usage_events (
  thread_id,
  runtime,
  agent,
  model_provider,
  model,
  input_tokens,
  cached_input_tokens,
  cached_write_tokens,
  output_tokens,
  reasoning_output_tokens,
  total_tokens,
  context_tokens,
  context_window_tokens,
  source,
  created_at_ms
)
SELECT
  id,
  runtime,
  coalesce(acp_agent, ''),
  coalesce(model_provider, ''),
  coalesce(model, ''),
  input_tokens,
  cached_input_tokens,
  cached_write_tokens,
  output_tokens,
  reasoning_output_tokens,
  total_tokens,
  context_tokens,
  context_window_tokens,
  'session_import',
  updated_at_ms
FROM threads
WHERE NOT EXISTS (
  SELECT 1 FROM usage_events WHERE usage_events.thread_id = threads.id
)
AND (
  input_tokens > 0 OR
  cached_input_tokens > 0 OR
  cached_write_tokens > 0 OR
  output_tokens > 0 OR
  reasoning_output_tokens > 0 OR
  total_tokens > 0
);

-- +goose Down
DROP TABLE IF EXISTS usage_events;
