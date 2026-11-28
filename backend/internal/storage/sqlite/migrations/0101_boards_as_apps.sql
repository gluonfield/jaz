-- +goose Up
-- A board becomes an app that holds other apps. Its tiles reference apps, and
-- a loop has its app from the moment it is put on a board.

CREATE TABLE migrating_boards (board_id TEXT PRIMARY KEY, app_id TEXT NOT NULL);
INSERT INTO migrating_boards
SELECT id, lower(hex(randomblob(13))) FROM boards;

CREATE TABLE migrating_tiles (widget_id TEXT PRIMARY KEY, loop_id TEXT NOT NULL, app_id TEXT NOT NULL);
INSERT INTO migrating_tiles
SELECT w.id, w.loop_id, COALESCE(a.id, lower(hex(randomblob(13))))
FROM widgets w
JOIN loops l ON l.id = w.loop_id AND l.status <> 'deleted'
LEFT JOIN apps a ON a.loop_id = w.loop_id AND a.status = 'active';

CREATE TABLE apps_new (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL DEFAULT 'local',
  source_thread_id TEXT NOT NULL DEFAULT '',
  title TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  icon_json TEXT NOT NULL DEFAULT '{}',
  kind TEXT NOT NULL DEFAULT 'artifact' CHECK (kind IN ('artifact', 'document', 'board')),
  visibility TEXT NOT NULL DEFAULT 'private' CHECK (visibility IN ('private', 'selected', 'space', 'team')),
  current_version INTEGER NOT NULL DEFAULT 0,
  status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'deleting')),
  created_at_ms INTEGER NOT NULL,
  updated_at_ms INTEGER NOT NULL
);
INSERT INTO apps_new (id, workspace_id, source_thread_id, title, description, icon_json, kind, visibility, current_version, status, created_at_ms, updated_at_ms)
SELECT id, workspace_id, source_thread_id, title, description, icon_json, kind, visibility, current_version, status, created_at_ms, updated_at_ms
FROM apps;
INSERT INTO apps_new (id, workspace_id, title, kind, created_at_ms, updated_at_ms)
SELECT m.app_id, b.workspace_id, b.name, 'board', b.created_at_ms, b.updated_at_ms
FROM boards b
JOIN migrating_boards m ON m.board_id = b.id;
INSERT INTO apps_new (id, title, created_at_ms, updated_at_ms)
SELECT m.app_id, COALESCE(NULLIF(w.title, ''), l.name), w.created_at_ms, w.updated_at_ms
FROM migrating_tiles m
JOIN widgets w ON w.id = m.widget_id
JOIN loops l ON l.id = m.loop_id
WHERE m.app_id NOT IN (SELECT id FROM apps);

CREATE TABLE app_versions_new (
  app_id TEXT NOT NULL REFERENCES apps_new(id) ON DELETE CASCADE,
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
INSERT INTO app_versions_new (app_id, version, bundle_sha256, entry_path, storage_dir, size_bytes, file_count, manifest_json, capabilities_json, allowed_tools_json, external_urls_json, files_json, created_at_ms)
SELECT app_id, version, bundle_sha256, entry_path, storage_dir, size_bytes, file_count, manifest_json, capabilities_json, allowed_tools_json, external_urls_json, files_json, created_at_ms
FROM app_versions;

CREATE TABLE app_kv_new (
  app_id TEXT NOT NULL REFERENCES apps_new(id) ON DELETE CASCADE,
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
INSERT INTO app_kv_new (app_id, scope_type, scope_owner_id, key, value_text, revision, size_bytes, updated_by, updated_at_ms)
SELECT app_id, scope_type, scope_owner_id, key, value_text, revision, size_bytes, updated_by, updated_at_ms
FROM app_kv;

-- The app a loop keeps on its boards, with what its next run needs to know.
CREATE TABLE loop_apps (
  loop_id TEXT PRIMARY KEY,
  app_id TEXT NOT NULL UNIQUE REFERENCES apps_new(id) ON DELETE CASCADE,
  size_hint TEXT NOT NULL DEFAULT '',
  last_error TEXT NOT NULL DEFAULT '',
  last_layout TEXT NOT NULL DEFAULT '',
  published_run_id TEXT NOT NULL DEFAULT ''
);
INSERT INTO loop_apps (loop_id, app_id, size_hint, last_error, last_layout, published_run_id)
SELECT m.loop_id, m.app_id, w.size_hint, COALESCE(w.last_error, ''), w.last_layout, w.published_run_id
FROM migrating_tiles m
JOIN widgets w ON w.id = m.widget_id;
INSERT OR IGNORE INTO loop_apps (loop_id, app_id)
SELECT a.loop_id, a.id
FROM apps a
JOIN loops l ON l.id = a.loop_id AND l.status <> 'deleted'
WHERE a.status = 'active';

CREATE TABLE board_settings (
  board_id TEXT PRIMARY KEY REFERENCES apps_new(id) ON DELETE CASCADE,
  font_scale REAL NOT NULL DEFAULT 1
);
INSERT INTO board_settings (board_id, font_scale)
SELECT m.app_id, b.font_scale
FROM boards b
JOIN migrating_boards m ON m.board_id = b.id;

CREATE TABLE board_tiles (
  board_id TEXT NOT NULL REFERENCES apps_new(id) ON DELETE CASCADE,
  app_id TEXT NOT NULL REFERENCES apps_new(id) ON DELETE CASCADE,
  x INTEGER NOT NULL DEFAULT 0,
  y INTEGER NOT NULL DEFAULT 0,
  w INTEGER NOT NULL DEFAULT 2,
  h INTEGER NOT NULL DEFAULT 2,
  placed_by TEXT NOT NULL DEFAULT 'llm',
  created_at_ms INTEGER NOT NULL,
  updated_at_ms INTEGER NOT NULL,
  PRIMARY KEY (board_id, app_id)
);
INSERT OR IGNORE INTO board_tiles (board_id, app_id, x, y, w, h, placed_by, created_at_ms, updated_at_ms)
SELECT b.app_id, COALESCE(bw.app_id, t.app_id), bw.x, bw.y, bw.w, bw.h, bw.placed_by, bw.created_at_ms, bw.updated_at_ms
FROM board_widgets bw
JOIN migrating_boards b ON b.board_id = bw.board_id
LEFT JOIN migrating_tiles t ON t.widget_id = bw.widget_id
WHERE COALESCE(bw.app_id, t.app_id) IN (SELECT id FROM apps_new);
CREATE INDEX idx_board_tiles_app ON board_tiles (app_id);

-- HTML a loop published before tiles became apps, until startup converts it.
CREATE TABLE legacy_widget_html (
  app_id TEXT PRIMARY KEY REFERENCES apps_new(id) ON DELETE CASCADE,
  html TEXT NOT NULL
);
INSERT INTO legacy_widget_html (app_id, html)
SELECT m.app_id, v.html
FROM migrating_tiles m
JOIN widgets w ON w.id = m.widget_id
JOIN widget_versions v ON v.widget_id = w.id AND v.version = w.current_version;

DROP TABLE board_widgets;
DROP TABLE widget_versions;
DROP TABLE widgets;
DROP TABLE boards;
DROP TABLE app_versions;
DROP TABLE app_kv;
DROP TABLE apps;
DROP TABLE migrating_tiles;
DROP TABLE migrating_boards;

ALTER TABLE apps_new RENAME TO apps;
ALTER TABLE app_versions_new RENAME TO app_versions;
ALTER TABLE app_kv_new RENAME TO app_kv;
CREATE INDEX idx_apps_workspace_updated ON apps (workspace_id, updated_at_ms DESC);
CREATE INDEX idx_app_versions_sha ON app_versions (app_id, bundle_sha256);
CREATE INDEX idx_app_kv_app_scope ON app_kv (app_id, scope_type, scope_owner_id);

-- +goose Down
CREATE TABLE boards (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  grid_cols INTEGER NOT NULL DEFAULT 6,
  row_height INTEGER NOT NULL DEFAULT 120,
  window_bounds TEXT,
  is_default INTEGER NOT NULL DEFAULT 0,
  created_at_ms INTEGER NOT NULL,
  updated_at_ms INTEGER NOT NULL,
  font_scale REAL NOT NULL DEFAULT 1,
  workspace_id TEXT NOT NULL DEFAULT 'local'
);
INSERT INTO boards (id, name, created_at_ms, updated_at_ms, font_scale, workspace_id)
SELECT a.id, a.title, a.created_at_ms, a.updated_at_ms, COALESCE(s.font_scale, 1), a.workspace_id
FROM apps a
LEFT JOIN board_settings s ON s.board_id = a.id
WHERE a.kind = 'board';
CREATE INDEX idx_boards_workspace ON boards (workspace_id, created_at_ms ASC, id);

CREATE TABLE widgets (
  id TEXT PRIMARY KEY,
  loop_id TEXT NOT NULL UNIQUE,
  title TEXT NOT NULL DEFAULT '',
  current_version INTEGER NOT NULL DEFAULT 0,
  size_hint TEXT NOT NULL DEFAULT '',
  last_error TEXT,
  created_at_ms INTEGER NOT NULL,
  updated_at_ms INTEGER NOT NULL,
  last_layout TEXT NOT NULL DEFAULT '',
  workspace_id TEXT NOT NULL DEFAULT 'local',
  published_run_id TEXT NOT NULL DEFAULT '',
  FOREIGN KEY (loop_id) REFERENCES loops(id)
);
INSERT INTO widgets (id, loop_id, title, current_version, size_hint, last_error, created_at_ms, updated_at_ms, last_layout, published_run_id)
SELECT 'widget-' || la.app_id, la.loop_id, a.title, CASE WHEN h.app_id IS NULL THEN 0 ELSE 1 END,
  la.size_hint, NULLIF(la.last_error, ''), a.created_at_ms, a.updated_at_ms, la.last_layout, la.published_run_id
FROM loop_apps la
JOIN apps a ON a.id = la.app_id
JOIN loops l ON l.id = la.loop_id
LEFT JOIN legacy_widget_html h ON h.app_id = la.app_id;
CREATE INDEX idx_widgets_loop ON widgets(loop_id);
CREATE INDEX idx_widgets_workspace ON widgets (workspace_id, updated_at_ms DESC);

CREATE TABLE widget_versions (
  widget_id TEXT NOT NULL,
  version INTEGER NOT NULL,
  html TEXT NOT NULL,
  produced_by_run_id TEXT,
  created_at_ms INTEGER NOT NULL,
  PRIMARY KEY (widget_id, version),
  FOREIGN KEY (widget_id) REFERENCES widgets(id) ON DELETE CASCADE
);
INSERT INTO widget_versions (widget_id, version, html, created_at_ms)
SELECT w.id, 1, h.html, w.updated_at_ms
FROM legacy_widget_html h
JOIN widgets w ON w.id = 'widget-' || h.app_id;
CREATE INDEX idx_widget_versions_widget ON widget_versions(widget_id, version DESC);

CREATE TABLE apps_old (
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
  updated_at_ms INTEGER NOT NULL,
  loop_id TEXT NOT NULL DEFAULT ''
);
INSERT INTO apps_old (id, workspace_id, source_thread_id, title, description, icon_json, kind, visibility, current_version, status, created_at_ms, updated_at_ms, loop_id)
SELECT a.id, a.workspace_id, a.source_thread_id, a.title, a.description, a.icon_json, a.kind, a.visibility, a.current_version, a.status, a.created_at_ms, a.updated_at_ms, COALESCE(la.loop_id, '')
FROM apps a
LEFT JOIN loop_apps la ON la.app_id = a.id
WHERE a.kind <> 'board' AND (a.current_version > 0 OR la.app_id IS NULL);

CREATE TABLE app_versions_old (
  app_id TEXT NOT NULL REFERENCES apps_old(id) ON DELETE CASCADE,
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
INSERT INTO app_versions_old (app_id, version, bundle_sha256, entry_path, storage_dir, size_bytes, file_count, manifest_json, capabilities_json, allowed_tools_json, external_urls_json, files_json, created_at_ms)
SELECT app_id, version, bundle_sha256, entry_path, storage_dir, size_bytes, file_count, manifest_json, capabilities_json, allowed_tools_json, external_urls_json, files_json, created_at_ms
FROM app_versions;

CREATE TABLE app_kv_old (
  app_id TEXT NOT NULL REFERENCES apps_old(id) ON DELETE CASCADE,
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
INSERT INTO app_kv_old (app_id, scope_type, scope_owner_id, key, value_text, revision, size_bytes, updated_by, updated_at_ms)
SELECT app_id, scope_type, scope_owner_id, key, value_text, revision, size_bytes, updated_by, updated_at_ms
FROM app_kv
WHERE app_id IN (SELECT id FROM apps_old);

CREATE TABLE board_widgets (
  board_id TEXT NOT NULL REFERENCES boards(id) ON DELETE CASCADE,
  widget_id TEXT NOT NULL,
  app_id TEXT REFERENCES apps_old(id) ON DELETE CASCADE,
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
SELECT t.board_id,
  CASE WHEN la.app_id IS NULL THEN 'app_' || t.app_id ELSE 'widget-' || t.app_id END,
  CASE WHEN la.app_id IS NULL THEN t.app_id END,
  t.x, t.y, t.w, t.h, t.placed_by, t.created_at_ms, t.updated_at_ms
FROM board_tiles t
LEFT JOIN loop_apps la ON la.app_id = t.app_id;
CREATE INDEX idx_board_widgets_app ON board_widgets (app_id) WHERE app_id IS NOT NULL;

DROP TABLE legacy_widget_html;
DROP TABLE board_tiles;
DROP TABLE board_settings;
DROP TABLE loop_apps;
DROP TABLE app_versions;
DROP TABLE app_kv;
DROP TABLE apps;

ALTER TABLE apps_old RENAME TO apps;
ALTER TABLE app_versions_old RENAME TO app_versions;
ALTER TABLE app_kv_old RENAME TO app_kv;
CREATE INDEX idx_apps_workspace_updated ON apps (workspace_id, updated_at_ms DESC);
CREATE UNIQUE INDEX idx_apps_loop ON apps (loop_id) WHERE loop_id <> '' AND status = 'active';
CREATE INDEX idx_app_versions_sha ON app_versions (app_id, bundle_sha256);
CREATE INDEX idx_app_kv_app_scope ON app_kv (app_id, scope_type, scope_owner_id);
