-- +goose Up
CREATE TABLE local_instructions (
  scope TEXT NOT NULL,
  scope_id TEXT NOT NULL,
  kind TEXT NOT NULL,
  body_md TEXT NOT NULL DEFAULT '',
  version TEXT NOT NULL DEFAULT '',
  updated_at_ms INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (scope, scope_id, kind)
);

-- +goose Down
DROP TABLE IF EXISTS local_instructions;
