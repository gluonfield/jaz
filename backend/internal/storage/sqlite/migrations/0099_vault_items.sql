-- +goose Up
CREATE TABLE vault_items (
  id TEXT PRIMARY KEY,
  kind TEXT NOT NULL,
  label TEXT NOT NULL DEFAULT '',
  origins_json TEXT NOT NULL DEFAULT '[]',
  meta_json TEXT NOT NULL DEFAULT '{}',
  secret_encrypted TEXT NOT NULL,
  created_at_ms INTEGER NOT NULL,
  updated_at_ms INTEGER NOT NULL
);

CREATE INDEX idx_vault_items_kind_updated ON vault_items (kind, updated_at_ms DESC, id);

-- +goose Down
DROP INDEX IF EXISTS idx_vault_items_kind_updated;
DROP TABLE IF EXISTS vault_items;
