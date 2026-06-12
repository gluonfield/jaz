-- +goose Up
CREATE TABLE IF NOT EXISTS integration_oauth_tokens (
  connection_id TEXT PRIMARY KEY,
  token_json TEXT NOT NULL,
  updated_at_ms INTEGER NOT NULL
);

INSERT OR REPLACE INTO integration_oauth_tokens (
  connection_id,
  token_json,
  updated_at_ms
)
SELECT
  'mcp:' || server_id,
  token_json,
  updated_at_ms
FROM mcp_oauth_tokens;

-- +goose Down
DROP TABLE IF EXISTS integration_oauth_tokens;
