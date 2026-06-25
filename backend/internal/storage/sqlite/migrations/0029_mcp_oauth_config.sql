-- +goose Up
ALTER TABLE mcp_servers ADD COLUMN oauth_json TEXT NOT NULL DEFAULT '{}';

-- +goose Down
ALTER TABLE mcp_servers DROP COLUMN oauth_json;
