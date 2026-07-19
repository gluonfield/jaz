-- +goose Up
CREATE INDEX idx_threads_listing
ON threads(archived, source_type, parent_id, last_attention_at_ms DESC, id);

-- +goose Down
DROP INDEX idx_threads_listing;
