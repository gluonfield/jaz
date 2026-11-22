-- +goose Up
CREATE TABLE plugin_sources (
  id TEXT PRIMARY KEY,
  repository TEXT NOT NULL,
  url TEXT NOT NULL,
  ref TEXT NOT NULL DEFAULT '',
  paths TEXT NOT NULL DEFAULT '[]',
  commit_sha TEXT NOT NULL DEFAULT '',
  auto_update INTEGER NOT NULL DEFAULT 0,
  is_marketplace INTEGER NOT NULL DEFAULT 0,
  catalog_json TEXT NOT NULL DEFAULT '[]',
  checked_at_ms INTEGER NOT NULL DEFAULT 0,
  updated_at_ms INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE plugin_state (
  plugin_id TEXT PRIMARY KEY,
  enabled INTEGER NOT NULL DEFAULT 1,
  source_id TEXT NOT NULL DEFAULT '',
  source_repository TEXT NOT NULL DEFAULT '',
  source_ref TEXT NOT NULL DEFAULT '',
  source_path TEXT NOT NULL DEFAULT '',
  commit_sha TEXT NOT NULL DEFAULT '',
  remote_sha TEXT NOT NULL DEFAULT '',
  config_json TEXT NOT NULL DEFAULT '{}',
  updated_at_ms INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE thread_plugin_state (
  thread_id TEXT NOT NULL,
  plugin_id TEXT NOT NULL,
  view TEXT NOT NULL,
  state_key TEXT NOT NULL DEFAULT '',
  data_json TEXT NOT NULL DEFAULT 'null',
  ui_state_json TEXT NOT NULL DEFAULT 'null',
  revision INTEGER NOT NULL DEFAULT 1,
  updated_at_ms INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (thread_id, plugin_id, view, state_key)
);

CREATE INDEX idx_thread_plugin_state_plugin ON thread_plugin_state (plugin_id, updated_at_ms DESC);

ALTER TABLE bots ADD COLUMN plugins TEXT NOT NULL DEFAULT 'null';

-- +goose Down
ALTER TABLE bots DROP COLUMN plugins;
DROP INDEX IF EXISTS idx_thread_plugin_state_plugin;
DROP TABLE IF EXISTS thread_plugin_state;
DROP TABLE IF EXISTS plugin_state;
DROP TABLE IF EXISTS plugin_sources;
