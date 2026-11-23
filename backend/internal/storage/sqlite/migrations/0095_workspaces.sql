-- +goose Up
CREATE TABLE cloud_accounts (
  id TEXT PRIMARY KEY,
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

INSERT INTO cloud_accounts (
  id,
  base_url,
  token_encrypted,
  oidc_login_url,
  user_id,
  org_id,
  org_name,
  email,
  display_name,
  role,
  capabilities_json,
  connected_at_ms,
  last_sync_at_ms,
  last_error
)
SELECT
  CASE
    WHEN user_id != '' AND org_id != '' THEN org_id || ':' || user_id || '@' || rtrim(base_url, '/')
    WHEN org_id != '' THEN org_id || '@' || rtrim(base_url, '/')
    ELSE rtrim(base_url, '/')
  END,
  base_url,
  token_encrypted,
  oidc_login_url,
  user_id,
  org_id,
  org_name,
  email,
  display_name,
  role,
  capabilities_json,
  connected_at_ms,
  last_sync_at_ms,
  last_error
FROM cloud_account;

INSERT INTO settings (namespace, key, value_json, created_at_ms, updated_at_ms)
SELECT
  'cloud',
  'active_workspace_id',
  json_quote(id),
  connected_at_ms,
  connected_at_ms
FROM cloud_accounts
LIMIT 1
ON CONFLICT(namespace, key) DO NOTHING;

DROP TABLE cloud_account;

-- Scope cloud_spaces by account_id.
ALTER TABLE cloud_spaces RENAME TO cloud_spaces_legacy;

CREATE TABLE cloud_spaces (
  account_id TEXT NOT NULL DEFAULT '',
  id TEXT NOT NULL,
  org_id TEXT NOT NULL,
  name TEXT NOT NULL,
  created_by TEXT NOT NULL DEFAULT '',
  created_at_ms INTEGER NOT NULL DEFAULT 0,
  updated_at_ms INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (account_id, id)
);

INSERT INTO cloud_spaces (account_id, id, org_id, name, created_by, created_at_ms, updated_at_ms)
SELECT
  COALESCE((SELECT id FROM cloud_accounts LIMIT 1), ''),
  id,
  org_id,
  name,
  created_by,
  created_at_ms,
  updated_at_ms
FROM cloud_spaces_legacy;

DROP TABLE cloud_spaces_legacy;

-- Scope cloud_threads and cloud_thread_events by account_id.
DROP INDEX IF EXISTS idx_cloud_threads_updated;
ALTER TABLE cloud_thread_events RENAME TO cloud_thread_events_legacy;
ALTER TABLE cloud_threads RENAME TO cloud_threads_legacy;

CREATE TABLE cloud_threads (
  account_id TEXT NOT NULL DEFAULT '',
  id TEXT NOT NULL,
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
  updated_at_ms INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (account_id, id)
);

CREATE INDEX idx_cloud_threads_updated ON cloud_threads (account_id, updated_at_ms DESC, id ASC);

INSERT INTO cloud_threads (
  account_id,
  id,
  org_id,
  owner_user_id,
  space_id,
  run_id,
  visibility,
  source_type,
  source_id,
  title,
  status,
  turn_epoch,
  model_profile_id,
  model,
  reasoning_effort,
  executor_target,
  runtime_ref_json,
  personal_sources_enabled,
  pinned,
  archived,
  unread,
  last_seq,
  created_at_ms,
  updated_at_ms
)
SELECT
  COALESCE((SELECT id FROM cloud_accounts LIMIT 1), ''),
  id,
  org_id,
  owner_user_id,
  space_id,
  run_id,
  visibility,
  source_type,
  source_id,
  title,
  status,
  turn_epoch,
  model_profile_id,
  model,
  reasoning_effort,
  executor_target,
  runtime_ref_json,
  personal_sources_enabled,
  pinned,
  archived,
  unread,
  last_seq,
  created_at_ms,
  updated_at_ms
FROM cloud_threads_legacy;

CREATE TABLE cloud_thread_events (
  account_id TEXT NOT NULL DEFAULT '',
  thread_id TEXT NOT NULL,
  seq INTEGER NOT NULL,
  org_id TEXT NOT NULL DEFAULT '',
  turn_epoch INTEGER NOT NULL DEFAULT 0,
  actor_user_id TEXT NOT NULL DEFAULT '',
  type TEXT NOT NULL,
  payload_json TEXT NOT NULL DEFAULT '{}',
  created_at_ms INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (account_id, thread_id, seq),
  FOREIGN KEY (account_id, thread_id) REFERENCES cloud_threads(account_id, id) ON DELETE CASCADE
);

INSERT INTO cloud_thread_events (
  account_id,
  thread_id,
  seq,
  org_id,
  turn_epoch,
  actor_user_id,
  type,
  payload_json,
  created_at_ms
)
SELECT
  COALESCE((SELECT id FROM cloud_accounts LIMIT 1), ''),
  thread_id,
  seq,
  org_id,
  turn_epoch,
  actor_user_id,
  type,
  payload_json,
  created_at_ms
FROM cloud_thread_events_legacy;

DROP TABLE cloud_thread_events_legacy;
DROP TABLE cloud_threads_legacy;

-- Scope cloud_instructions by account_id.
ALTER TABLE cloud_instructions RENAME TO cloud_instructions_legacy;

CREATE TABLE cloud_instructions (
  account_id TEXT NOT NULL DEFAULT '',
  org_id TEXT NOT NULL,
  scope TEXT NOT NULL,
  scope_id TEXT NOT NULL,
  kind TEXT NOT NULL,
  body_md TEXT NOT NULL DEFAULT '',
  version TEXT NOT NULL DEFAULT '',
  updated_by TEXT NOT NULL DEFAULT '',
  updated_at_ms INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (account_id, org_id, scope, scope_id, kind)
);

INSERT INTO cloud_instructions (
  account_id,
  org_id,
  scope,
  scope_id,
  kind,
  body_md,
  version,
  updated_by,
  updated_at_ms
)
SELECT
  COALESCE((SELECT id FROM cloud_accounts LIMIT 1), ''),
  org_id,
  scope,
  scope_id,
  kind,
  body_md,
  version,
  updated_by,
  updated_at_ms
FROM cloud_instructions_legacy;

DROP TABLE cloud_instructions_legacy;

-- Scope cloud_usage_events by account_id.
DROP INDEX IF EXISTS idx_cloud_usage_events_created_at;
ALTER TABLE cloud_usage_events RENAME TO cloud_usage_events_legacy;

CREATE TABLE cloud_usage_events (
  account_id TEXT NOT NULL DEFAULT '',
  id INTEGER NOT NULL,
  org_id TEXT NOT NULL,
  user_id TEXT NOT NULL DEFAULT '',
  thread_id TEXT NOT NULL,
  model_profile_id TEXT NOT NULL DEFAULT '',
  model TEXT NOT NULL DEFAULT '',
  input_tokens INTEGER NOT NULL DEFAULT 0,
  cached_input_tokens INTEGER NOT NULL DEFAULT 0,
  output_tokens INTEGER NOT NULL DEFAULT 0,
  cost_usd REAL NOT NULL DEFAULT 0,
  created_at_ms INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (account_id, id)
);

CREATE INDEX idx_cloud_usage_events_created_at ON cloud_usage_events (account_id, created_at_ms);

INSERT INTO cloud_usage_events (
  account_id,
  id,
  org_id,
  user_id,
  thread_id,
  model_profile_id,
  model,
  input_tokens,
  cached_input_tokens,
  output_tokens,
  cost_usd,
  created_at_ms
)
SELECT
  COALESCE((SELECT id FROM cloud_accounts LIMIT 1), ''),
  id,
  org_id,
  user_id,
  thread_id,
  model_profile_id,
  model,
  input_tokens,
  cached_input_tokens,
  output_tokens,
  cost_usd,
  created_at_ms
FROM cloud_usage_events_legacy;

DROP TABLE cloud_usage_events_legacy;

-- Scope local threads and usage_events by workspace_id (existing rows stay in 'local').
ALTER TABLE threads ADD COLUMN workspace_id TEXT NOT NULL DEFAULT 'local';
CREATE INDEX idx_threads_workspace_listing ON threads (workspace_id, archived, source_type, parent_id, last_attention_at_ms DESC, id);

ALTER TABLE usage_events ADD COLUMN workspace_id TEXT NOT NULL DEFAULT 'local';
CREATE INDEX idx_usage_events_workspace_created_at ON usage_events (workspace_id, created_at_ms);

-- +goose Down
DROP INDEX IF EXISTS idx_usage_events_workspace_created_at;
ALTER TABLE usage_events DROP COLUMN workspace_id;

DROP INDEX IF EXISTS idx_threads_workspace_listing;
ALTER TABLE threads DROP COLUMN workspace_id;

DELETE FROM settings WHERE namespace = 'cloud' AND key = 'active_workspace_id';

DROP INDEX IF EXISTS idx_cloud_usage_events_created_at;
DROP TABLE IF EXISTS cloud_usage_events;
DROP TABLE IF EXISTS cloud_instructions;
DROP TABLE IF EXISTS cloud_thread_events;
DROP INDEX IF EXISTS idx_cloud_threads_updated;
DROP TABLE IF EXISTS cloud_threads;
DROP TABLE IF EXISTS cloud_spaces;
DROP TABLE IF EXISTS cloud_accounts;

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
