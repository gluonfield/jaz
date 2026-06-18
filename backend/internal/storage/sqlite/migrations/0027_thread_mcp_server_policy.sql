-- +goose Up
ALTER TABLE threads ADD COLUMN mcp_server_policy TEXT;

-- +goose Down
ALTER TABLE threads DROP COLUMN mcp_server_policy;
