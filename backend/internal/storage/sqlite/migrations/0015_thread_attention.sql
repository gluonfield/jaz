-- +goose Up
ALTER TABLE threads ADD COLUMN last_attention_at_ms INTEGER NOT NULL DEFAULT 0;

UPDATE threads
SET last_attention_at_ms = CASE
  WHEN updated_at_ms > 0 THEN updated_at_ms
  ELSE created_at_ms
END
WHERE last_attention_at_ms = 0;

-- +goose Down
ALTER TABLE threads DROP COLUMN last_attention_at_ms;
