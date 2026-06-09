-- +goose Up
CREATE TABLE IF NOT EXISTS settings (
  namespace TEXT NOT NULL,
  key TEXT NOT NULL,
  value_json TEXT NOT NULL,
  created_at_ms INTEGER NOT NULL,
  updated_at_ms INTEGER NOT NULL,
  PRIMARY KEY (namespace, key)
);

-- +goose Down
DROP TABLE IF EXISTS settings;
