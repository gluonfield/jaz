-- +goose Up
DROP INDEX IF EXISTS idx_usage_events_thread_created;

-- +goose Down
CREATE INDEX IF NOT EXISTS idx_usage_events_thread_created ON usage_events(thread_id, created_at_ms);
