-- +goose Up
ALTER TABLE threads ADD COLUMN last_seen_at_ms INTEGER NOT NULL DEFAULT 0;

-- Seed existing threads as already-seen up to their last attention moment so the
-- Feed starts empty; only genuinely newer messages resurface them.
UPDATE threads
SET last_seen_at_ms = CASE
  WHEN last_attention_at_ms > 0 THEN last_attention_at_ms
  WHEN updated_at_ms > 0 THEN updated_at_ms
  ELSE created_at_ms
END
WHERE last_seen_at_ms = 0;

-- +goose Down
ALTER TABLE threads DROP COLUMN last_seen_at_ms;
