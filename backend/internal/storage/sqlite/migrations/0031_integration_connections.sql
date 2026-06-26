-- +goose Up
CREATE TABLE IF NOT EXISTS integration_connections (
  id TEXT PRIMARY KEY,
  provider TEXT NOT NULL,
  account_id TEXT NOT NULL,
  account_name TEXT NOT NULL DEFAULT '',
  alias TEXT NOT NULL DEFAULT '',
  scopes_json TEXT NOT NULL DEFAULT '[]',
  updated_at_ms INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_integration_connections_provider
ON integration_connections(provider, id);

-- +goose Down
DROP INDEX IF EXISTS idx_integration_connections_provider;
DROP TABLE IF EXISTS integration_connections;
