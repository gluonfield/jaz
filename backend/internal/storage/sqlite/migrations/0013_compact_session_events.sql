-- +goose Up
-- Old rows repeated the session title and mode catalog on every event (most
-- of the payload on tool-heavy threads). New rows no longer carry them;
-- /messages serves the labels once per response via acp_meta. The slug stays
-- embedded as a durable label fallback.
-- json_valid guards legacy ''/non-JSON values (eventFromDB tolerates them on
-- read, but json_remove would raise "malformed JSON" and abort the migration).
UPDATE session_events
SET acp = json_remove(acp, '$.title', '$.modes.available_modes')
WHERE acp IS NOT NULL AND json_valid(acp);

-- Plan approval reads current/plan mode ids; every other row drops modes.
UPDATE session_events
SET acp = json_remove(acp, '$.modes')
WHERE acp IS NOT NULL AND json_valid(acp) AND json_extract(acp, '$.plan') IS NULL;

-- +goose Down
-- The removed fields were duplicates of thread metadata; nothing to restore.
