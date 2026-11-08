-- +goose Up
ALTER TABLE mcp_servers ADD COLUMN stdio_json TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE mcp_servers DROP COLUMN stdio_json;
