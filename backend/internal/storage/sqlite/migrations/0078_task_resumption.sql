-- +goose Up
DROP VIEW bot_commitment_watch;
ALTER TABLE bot_commitments ADD COLUMN reply_seq INTEGER NOT NULL DEFAULT 0;
ALTER TABLE bot_commitments ADD COLUMN check_at_ms INTEGER NOT NULL DEFAULT 0;

CREATE VIEW bot_commitment_watch AS
SELECT c.id, c.bot_id, c.wake_progress, c.wake_attempts, c.wake_at_ms, c.check_at_ms, p.progress
FROM bot_commitments c
JOIN bot_commitment_progress p ON p.commitment_id = c.id
JOIN threads b ON b.id = c.bot_id
WHERE c.status IN ('pending', 'in_progress') AND NOT p.moving AND NOT p.blocked
  AND b.archived = 0 AND b.background_work = 0 AND b.status IN ('idle', 'error') AND b.turn = ''
  AND json_array_length(COALESCE(NULLIF(b.queued_messages, ''), '[]')) = 0;

-- +goose Down
DROP VIEW bot_commitment_watch;
ALTER TABLE bot_commitments DROP COLUMN reply_seq;
ALTER TABLE bot_commitments DROP COLUMN check_at_ms;

CREATE VIEW bot_commitment_watch AS
SELECT c.id, c.bot_id, c.wake_progress, c.wake_attempts, c.wake_at_ms, p.progress
FROM bot_commitments c
JOIN bot_commitment_progress p ON p.commitment_id = c.id
JOIN threads b ON b.id = c.bot_id
WHERE c.status IN ('pending', 'in_progress') AND NOT p.moving AND NOT p.blocked
  AND b.archived = 0 AND b.background_work = 0 AND b.status IN ('idle', 'error') AND b.turn = ''
  AND json_array_length(COALESCE(NULLIF(b.queued_messages, ''), '[]')) = 0;
