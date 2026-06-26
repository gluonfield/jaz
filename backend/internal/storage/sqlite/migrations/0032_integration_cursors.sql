-- +goose Up
CREATE TABLE IF NOT EXISTS integration_cursors (
  connection_id TEXT NOT NULL,
  kind TEXT NOT NULL,
  value_json TEXT NOT NULL DEFAULT '{}',
  updated_at_ms INTEGER NOT NULL,
  PRIMARY KEY(connection_id, kind),
  FOREIGN KEY(connection_id) REFERENCES integration_connections(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_integration_cursors_connection
ON integration_cursors(connection_id);

-- +goose Down
DROP INDEX IF EXISTS idx_integration_cursors_connection;
DROP TABLE IF EXISTS integration_cursors;
