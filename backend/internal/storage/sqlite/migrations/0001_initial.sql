-- +goose Up
CREATE TABLE IF NOT EXISTS threads (
  id TEXT PRIMARY KEY,
  slug TEXT NOT NULL UNIQUE,
  title TEXT,
  parent_id TEXT,
  status TEXT NOT NULL DEFAULT 'idle',
  error TEXT,
  runtime TEXT NOT NULL DEFAULT 'acp',
  acp_agent TEXT,
  acp_session_id TEXT,
  cwd TEXT,
  model_provider TEXT,
  model TEXT,
  reasoning_effort TEXT,
  input_tokens INTEGER NOT NULL DEFAULT 0,
  cached_input_tokens INTEGER NOT NULL DEFAULT 0,
  output_tokens INTEGER NOT NULL DEFAULT 0,
  reasoning_output_tokens INTEGER NOT NULL DEFAULT 0,
  total_tokens INTEGER NOT NULL DEFAULT 0,
  queued_messages TEXT NOT NULL DEFAULT '[]',
  source_type TEXT,
  source_id TEXT,
  archived INTEGER NOT NULL DEFAULT 0,
  created_at_ms INTEGER NOT NULL,
  updated_at_ms INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS messages (
  thread_id TEXT NOT NULL,
  seq INTEGER NOT NULL,
  role TEXT NOT NULL,
  content TEXT NOT NULL DEFAULT '',
  reasoning TEXT,
  blocks TEXT NOT NULL DEFAULT '[]',
  created_at_ms INTEGER NOT NULL,
  PRIMARY KEY (thread_id, seq),
  FOREIGN KEY (thread_id) REFERENCES threads(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS session_events (
  thread_id TEXT NOT NULL,
  seq INTEGER NOT NULL,
  type TEXT NOT NULL,
  content TEXT NOT NULL DEFAULT '',
  acp TEXT,
  permission TEXT,
  created_at_ms INTEGER NOT NULL,
  PRIMARY KEY (thread_id, seq),
  FOREIGN KEY (thread_id) REFERENCES threads(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS mcp_servers (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  transport TEXT NOT NULL DEFAULT 'streamable_http',
  url TEXT NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 1,
  bearer_token_env_var TEXT,
  headers_json TEXT NOT NULL DEFAULT '[]',
  env_headers_json TEXT NOT NULL DEFAULT '[]',
  created_at_ms INTEGER NOT NULL,
  updated_at_ms INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS mcp_oauth_tokens (
  server_id TEXT PRIMARY KEY,
  token_json TEXT NOT NULL,
  updated_at_ms INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS loops (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  prompt TEXT NOT NULL,
  schedule_kind TEXT NOT NULL,
  schedule_expr TEXT NOT NULL,
  timezone TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'active',
  runtime TEXT NOT NULL DEFAULT 'acp',
  acp_agent TEXT,
  next_run_at_ms INTEGER NOT NULL DEFAULT 0,
  last_run_at_ms INTEGER NOT NULL DEFAULT 0,
  last_run_id TEXT,
  last_run_thread_id TEXT,
  last_run_status TEXT,
  last_error TEXT,
  created_at_ms INTEGER NOT NULL,
  updated_at_ms INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS loop_runs (
  id TEXT PRIMARY KEY,
  loop_id TEXT NOT NULL,
  thread_id TEXT,
  scheduled_for_ms INTEGER NOT NULL,
  started_at_ms INTEGER NOT NULL DEFAULT 0,
  finished_at_ms INTEGER NOT NULL DEFAULT 0,
  status TEXT NOT NULL,
  error TEXT,
  created_at_ms INTEGER NOT NULL,
  FOREIGN KEY (loop_id) REFERENCES loops(id),
  FOREIGN KEY (thread_id) REFERENCES threads(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_threads_parent_updated ON threads(parent_id, updated_at_ms DESC);
CREATE INDEX IF NOT EXISTS idx_threads_updated ON threads(updated_at_ms DESC);
CREATE INDEX IF NOT EXISTS idx_mcp_servers_updated ON mcp_servers(updated_at_ms DESC);
CREATE INDEX IF NOT EXISTS idx_loops_next_run ON loops(status, next_run_at_ms);
CREATE INDEX IF NOT EXISTS idx_loop_runs_loop_created ON loop_runs(loop_id, created_at_ms DESC);
CREATE INDEX IF NOT EXISTS idx_loop_runs_thread ON loop_runs(thread_id);

-- +goose Down
DROP TABLE IF EXISTS loop_runs;
DROP TABLE IF EXISTS loops;
DROP TABLE IF EXISTS mcp_oauth_tokens;
DROP TABLE IF EXISTS mcp_servers;
DROP TABLE IF EXISTS session_events;
DROP TABLE IF EXISTS messages;
DROP TABLE IF EXISTS threads;
