-- +goose Up
CREATE TABLE cloud_account (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  base_url TEXT NOT NULL,
  token_encrypted TEXT NOT NULL DEFAULT '',
  oidc_login_url TEXT NOT NULL DEFAULT '',
  user_id TEXT NOT NULL DEFAULT '',
  org_id TEXT NOT NULL DEFAULT '',
  org_name TEXT NOT NULL DEFAULT '',
  email TEXT NOT NULL DEFAULT '',
  display_name TEXT NOT NULL DEFAULT '',
  role TEXT NOT NULL DEFAULT '',
  capabilities_json TEXT NOT NULL DEFAULT '{}',
  connected_at_ms INTEGER NOT NULL DEFAULT 0,
  last_sync_at_ms INTEGER NOT NULL DEFAULT 0,
  last_error TEXT NOT NULL DEFAULT ''
);

CREATE TABLE cloud_spaces (
  id TEXT PRIMARY KEY,
  org_id TEXT NOT NULL,
  name TEXT NOT NULL,
  created_by TEXT NOT NULL DEFAULT '',
  created_at_ms INTEGER NOT NULL DEFAULT 0,
  updated_at_ms INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE cloud_threads (
  id TEXT PRIMARY KEY,
  org_id TEXT NOT NULL,
  owner_user_id TEXT NOT NULL DEFAULT '',
  space_id TEXT NOT NULL DEFAULT '',
  run_id TEXT NOT NULL,
  visibility TEXT NOT NULL DEFAULT 'private',
  source_type TEXT NOT NULL DEFAULT '',
  source_id TEXT NOT NULL DEFAULT '',
  title TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'idle',
  turn_epoch INTEGER NOT NULL DEFAULT 0,
  model_profile_id TEXT NOT NULL DEFAULT '',
  model TEXT NOT NULL DEFAULT '',
  reasoning_effort TEXT NOT NULL DEFAULT '',
  executor_target TEXT NOT NULL DEFAULT '',
  runtime_ref_json TEXT NOT NULL DEFAULT '{}',
  personal_sources_enabled INTEGER NOT NULL DEFAULT 0,
  pinned INTEGER NOT NULL DEFAULT 0,
  archived INTEGER NOT NULL DEFAULT 0,
  unread INTEGER NOT NULL DEFAULT 0,
  last_seq INTEGER NOT NULL DEFAULT 0,
  created_at_ms INTEGER NOT NULL DEFAULT 0,
  updated_at_ms INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX idx_cloud_threads_updated ON cloud_threads (updated_at_ms DESC, id ASC);

CREATE TABLE cloud_thread_events (
  thread_id TEXT NOT NULL,
  seq INTEGER NOT NULL,
  org_id TEXT NOT NULL DEFAULT '',
  turn_epoch INTEGER NOT NULL DEFAULT 0,
  actor_user_id TEXT NOT NULL DEFAULT '',
  type TEXT NOT NULL,
  payload_json TEXT NOT NULL DEFAULT '{}',
  created_at_ms INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (thread_id, seq),
  FOREIGN KEY (thread_id) REFERENCES cloud_threads(id) ON DELETE CASCADE
);

CREATE TABLE cloud_instructions (
  org_id TEXT NOT NULL,
  scope TEXT NOT NULL,
  scope_id TEXT NOT NULL,
  kind TEXT NOT NULL,
  body_md TEXT NOT NULL DEFAULT '',
  version TEXT NOT NULL DEFAULT '',
  updated_by TEXT NOT NULL DEFAULT '',
  updated_at_ms INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (org_id, scope, scope_id, kind)
);

CREATE TABLE cloud_usage_events (
  id INTEGER PRIMARY KEY,
  org_id TEXT NOT NULL,
  user_id TEXT NOT NULL DEFAULT '',
  thread_id TEXT NOT NULL,
  model_profile_id TEXT NOT NULL DEFAULT '',
  model TEXT NOT NULL DEFAULT '',
  input_tokens INTEGER NOT NULL DEFAULT 0,
  cached_input_tokens INTEGER NOT NULL DEFAULT 0,
  output_tokens INTEGER NOT NULL DEFAULT 0,
  cost_usd REAL NOT NULL DEFAULT 0,
  created_at_ms INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX idx_cloud_usage_events_created_at ON cloud_usage_events (created_at_ms);

-- +goose Down
DROP INDEX IF EXISTS idx_cloud_usage_events_created_at;
DROP TABLE IF EXISTS cloud_usage_events;
DROP TABLE IF EXISTS cloud_instructions;
DROP TABLE IF EXISTS cloud_thread_events;
DROP INDEX IF EXISTS idx_cloud_threads_updated;
DROP TABLE IF EXISTS cloud_threads;
DROP TABLE IF EXISTS cloud_spaces;
DROP TABLE IF EXISTS cloud_account;
