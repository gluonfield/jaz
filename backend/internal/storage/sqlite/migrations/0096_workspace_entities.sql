-- +goose Up
-- Scope bots by workspace_id; allow one main bot per workspace.
DROP INDEX IF EXISTS bots_one_main;
ALTER TABLE bots ADD COLUMN workspace_id TEXT NOT NULL DEFAULT 'local';
CREATE INDEX idx_bots_workspace ON bots (workspace_id, thread_id);
CREATE UNIQUE INDEX bots_one_main ON bots (workspace_id, main) WHERE main = 1;

-- Scope boards, widgets, loops, and custom MCP servers by workspace_id.
ALTER TABLE boards ADD COLUMN workspace_id TEXT NOT NULL DEFAULT 'local';
CREATE INDEX idx_boards_workspace ON boards (workspace_id, created_at_ms ASC, id);

ALTER TABLE widgets ADD COLUMN workspace_id TEXT NOT NULL DEFAULT 'local';
CREATE INDEX idx_widgets_workspace ON widgets (workspace_id, updated_at_ms DESC);

ALTER TABLE loops ADD COLUMN workspace_id TEXT NOT NULL DEFAULT 'local';
CREATE INDEX idx_loops_workspace ON loops (workspace_id, updated_at_ms DESC);

ALTER TABLE mcp_servers ADD COLUMN workspace_id TEXT NOT NULL DEFAULT 'local';
CREATE INDEX idx_mcp_servers_workspace ON mcp_servers (workspace_id, updated_at_ms DESC, id);

-- Scope plugin_sources and plugin_state with composite primary keys so
-- workspaces can independently add sources and install/toggle plugins.
ALTER TABLE plugin_sources RENAME TO plugin_sources_legacy;

CREATE TABLE plugin_sources (
  workspace_id TEXT NOT NULL DEFAULT 'local',
  id TEXT NOT NULL,
  repository TEXT NOT NULL,
  url TEXT NOT NULL,
  ref TEXT NOT NULL DEFAULT '',
  paths TEXT NOT NULL DEFAULT '[]',
  commit_sha TEXT NOT NULL DEFAULT '',
  auto_update INTEGER NOT NULL DEFAULT 0,
  is_marketplace INTEGER NOT NULL DEFAULT 0,
  catalog_json TEXT NOT NULL DEFAULT '[]',
  checked_at_ms INTEGER NOT NULL DEFAULT 0,
  updated_at_ms INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (workspace_id, id)
);

INSERT INTO plugin_sources (
  workspace_id,
  id,
  repository,
  url,
  ref,
  paths,
  commit_sha,
  auto_update,
  is_marketplace,
  catalog_json,
  checked_at_ms,
  updated_at_ms
)
SELECT
  'local',
  id,
  repository,
  url,
  ref,
  paths,
  commit_sha,
  auto_update,
  is_marketplace,
  catalog_json,
  checked_at_ms,
  updated_at_ms
FROM plugin_sources_legacy;

DROP TABLE plugin_sources_legacy;

ALTER TABLE plugin_state RENAME TO plugin_state_legacy;

CREATE TABLE plugin_state (
  workspace_id TEXT NOT NULL DEFAULT 'local',
  plugin_id TEXT NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 1,
  source_id TEXT NOT NULL DEFAULT '',
  source_repository TEXT NOT NULL DEFAULT '',
  source_ref TEXT NOT NULL DEFAULT '',
  source_path TEXT NOT NULL DEFAULT '',
  commit_sha TEXT NOT NULL DEFAULT '',
  remote_sha TEXT NOT NULL DEFAULT '',
  config_json TEXT NOT NULL DEFAULT '{}',
  updated_at_ms INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (workspace_id, plugin_id)
);

INSERT INTO plugin_state (
  workspace_id,
  plugin_id,
  enabled,
  source_id,
  source_repository,
  source_ref,
  source_path,
  commit_sha,
  remote_sha,
  config_json,
  updated_at_ms
)
SELECT
  'local',
  plugin_id,
  enabled,
  source_id,
  source_repository,
  source_ref,
  source_path,
  commit_sha,
  remote_sha,
  config_json,
  updated_at_ms
FROM plugin_state_legacy;

DROP TABLE plugin_state_legacy;

-- +goose Down
ALTER TABLE plugin_state RENAME TO plugin_state_ws;

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

INSERT INTO plugin_state (
  plugin_id,
  enabled,
  source_id,
  source_repository,
  source_ref,
  source_path,
  commit_sha,
  remote_sha,
  config_json,
  updated_at_ms
)
SELECT
  plugin_id,
  enabled,
  source_id,
  source_repository,
  source_ref,
  source_path,
  commit_sha,
  remote_sha,
  config_json,
  updated_at_ms
FROM plugin_state_ws
WHERE workspace_id = 'local';

DROP TABLE plugin_state_ws;

ALTER TABLE plugin_sources RENAME TO plugin_sources_ws;

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

INSERT INTO plugin_sources (
  id,
  repository,
  url,
  ref,
  paths,
  commit_sha,
  auto_update,
  is_marketplace,
  catalog_json,
  checked_at_ms,
  updated_at_ms
)
SELECT
  id,
  repository,
  url,
  ref,
  paths,
  commit_sha,
  auto_update,
  is_marketplace,
  catalog_json,
  checked_at_ms,
  updated_at_ms
FROM plugin_sources_ws
WHERE workspace_id = 'local';

DROP TABLE plugin_sources_ws;

DROP INDEX IF EXISTS idx_mcp_servers_workspace;
ALTER TABLE mcp_servers DROP COLUMN workspace_id;

DROP INDEX IF EXISTS idx_loops_workspace;
ALTER TABLE loops DROP COLUMN workspace_id;

DROP INDEX IF EXISTS idx_widgets_workspace;
ALTER TABLE widgets DROP COLUMN workspace_id;

DROP INDEX IF EXISTS idx_boards_workspace;
ALTER TABLE boards DROP COLUMN workspace_id;

DROP INDEX IF EXISTS bots_one_main;
DROP INDEX IF EXISTS idx_bots_workspace;
UPDATE bots SET main = 0 WHERE workspace_id <> 'local' AND main = 1;
ALTER TABLE bots DROP COLUMN workspace_id;
CREATE UNIQUE INDEX bots_one_main ON bots(main) WHERE main = 1;
