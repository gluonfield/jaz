-- +goose Up
CREATE INDEX idx_threads_attention ON threads(archived, last_attention_at_ms DESC, id);

-- +goose Down
DROP INDEX idx_threads_attention;
