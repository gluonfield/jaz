-- +goose Up
CREATE TABLE IF NOT EXISTS integration_oauth_tokens (
  connection_id TEXT PRIMARY KEY,
  token_json TEXT NOT NULL,
  updated_at_ms INTEGER NOT NULL
);

-- +goose Down
DROP TABLE IF EXISTS integration_oauth_tokens;
