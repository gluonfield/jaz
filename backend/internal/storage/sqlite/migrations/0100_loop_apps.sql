-- +goose Up
ALTER TABLE apps ADD COLUMN loop_id TEXT NOT NULL DEFAULT '';
CREATE UNIQUE INDEX IF NOT EXISTS idx_apps_loop ON apps (loop_id) WHERE loop_id <> '' AND status = 'active';
ALTER TABLE widgets ADD COLUMN published_run_id TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE widgets DROP COLUMN published_run_id;
DROP INDEX IF EXISTS idx_apps_loop;
ALTER TABLE apps DROP COLUMN loop_id;
