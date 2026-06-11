-- +goose Up
-- Old rows repeated the session title, slug, and mode catalog on every event
-- (most of the payload on tool-heavy threads). New rows no longer carry them;
-- /messages serves the labels once per response via acp_meta.
UPDATE session_events
SET acp = json_remove(acp, '$.title', '$.slug', '$.modes.available_modes')
WHERE acp IS NOT NULL;

-- Plan approval reads current/plan mode ids; every other row drops modes.
UPDATE session_events
SET acp = json_remove(acp, '$.modes')
WHERE acp IS NOT NULL AND json_extract(acp, '$.plan') IS NULL;

-- +goose Down
-- The removed fields were duplicates of thread metadata; nothing to restore.
