-- +goose Up

CREATE TABLE IF NOT EXISTS apps (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL DEFAULT 'local',
  source_thread_id TEXT NOT NULL DEFAULT '',
  title TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  icon_json TEXT NOT NULL DEFAULT '{}',
  kind TEXT NOT NULL DEFAULT 'artifact' CHECK (kind IN ('artifact', 'document')),
  visibility TEXT NOT NULL DEFAULT 'private' CHECK (visibility IN ('private', 'selected', 'space', 'team')),
  current_version INTEGER NOT NULL DEFAULT 0,
  status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'deleting')),
  created_at_ms INTEGER NOT NULL,
  updated_at_ms INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_apps_workspace_updated ON apps (workspace_id, updated_at_ms DESC);

CREATE TABLE IF NOT EXISTS app_versions (
  app_id TEXT NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
  version INTEGER NOT NULL,
  bundle_sha256 TEXT NOT NULL,
  entry_path TEXT NOT NULL,
  storage_dir TEXT NOT NULL,
  size_bytes INTEGER NOT NULL DEFAULT 0,
  file_count INTEGER NOT NULL DEFAULT 0,
  manifest_json TEXT NOT NULL DEFAULT '{}',
  capabilities_json TEXT NOT NULL DEFAULT '[]',
  allowed_tools_json TEXT NOT NULL DEFAULT '[]',
  external_urls_json TEXT NOT NULL DEFAULT '[]',
  files_json TEXT NOT NULL DEFAULT '[]',
  created_at_ms INTEGER NOT NULL,
  PRIMARY KEY (app_id, version)
);
CREATE INDEX IF NOT EXISTS idx_app_versions_sha ON app_versions (app_id, bundle_sha256);

CREATE TABLE IF NOT EXISTS app_drafts (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL DEFAULT 'local',
  target_app_id TEXT NOT NULL DEFAULT '',
  source_thread_id TEXT NOT NULL DEFAULT '',
  title TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  icon_json TEXT NOT NULL DEFAULT '{}',
  kind TEXT NOT NULL DEFAULT 'artifact' CHECK (kind IN ('artifact', 'document')),
  bundle_sha256 TEXT NOT NULL,
  entry_path TEXT NOT NULL,
  storage_dir TEXT NOT NULL,
  size_bytes INTEGER NOT NULL DEFAULT 0,
  file_count INTEGER NOT NULL DEFAULT 0,
  manifest_json TEXT NOT NULL DEFAULT '{}',
  capabilities_json TEXT NOT NULL DEFAULT '[]',
  allowed_tools_json TEXT NOT NULL DEFAULT '[]',
  external_urls_json TEXT NOT NULL DEFAULT '[]',
  warnings_json TEXT NOT NULL DEFAULT '[]',
  files_json TEXT NOT NULL DEFAULT '[]',
  expires_at_ms INTEGER NOT NULL,
  consumed_at_ms INTEGER,
  created_at_ms INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_app_drafts_expires ON app_drafts (expires_at_ms);

CREATE TABLE IF NOT EXISTS app_kv (
  app_id TEXT NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
  scope_type TEXT NOT NULL CHECK (scope_type IN ('personal', 'shared')),
  scope_owner_id TEXT NOT NULL,
  key TEXT NOT NULL,
  value_text TEXT NOT NULL,
  revision INTEGER NOT NULL DEFAULT 1,
  size_bytes INTEGER NOT NULL DEFAULT 0,
  updated_by TEXT NOT NULL DEFAULT '',
  updated_at_ms INTEGER NOT NULL,
  PRIMARY KEY (app_id, scope_type, scope_owner_id, key)
);
CREATE INDEX IF NOT EXISTS idx_app_kv_app_scope ON app_kv (app_id, scope_type, scope_owner_id);

-- Rebuild board_widgets to add nullable app_id and decouple widget_id from
-- requiring a row in widgets (so published apps can be pinned directly).
ALTER TABLE board_widgets RENAME TO board_widgets_legacy;

CREATE TABLE board_widgets (
  board_id TEXT NOT NULL REFERENCES boards(id) ON DELETE CASCADE,
  widget_id TEXT NOT NULL,
  app_id TEXT REFERENCES apps(id) ON DELETE CASCADE,
  x INTEGER NOT NULL DEFAULT 0,
  y INTEGER NOT NULL DEFAULT 0,
  w INTEGER NOT NULL DEFAULT 2,
  h INTEGER NOT NULL DEFAULT 2,
  placed_by TEXT NOT NULL DEFAULT 'llm',
  created_at_ms INTEGER NOT NULL,
  updated_at_ms INTEGER NOT NULL,
  PRIMARY KEY (board_id, widget_id)
);

INSERT INTO board_widgets (board_id, widget_id, app_id, x, y, w, h, placed_by, created_at_ms, updated_at_ms)
SELECT board_id, widget_id, NULL, x, y, w, h, placed_by, created_at_ms, updated_at_ms
FROM board_widgets_legacy;

DROP TABLE board_widgets_legacy;

CREATE INDEX IF NOT EXISTS idx_board_widgets_app ON board_widgets (app_id) WHERE app_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_board_widgets_app;
DROP TABLE IF EXISTS app_kv;
DROP TABLE IF EXISTS app_drafts;
DROP TABLE IF EXISTS app_versions;
DROP TABLE IF EXISTS apps;
