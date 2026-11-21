-- +goose Up
ALTER TABLE threads ADD COLUMN deferred_mcp_tools INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE threads DROP COLUMN deferred_mcp_tools;
