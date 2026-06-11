-- +goose Up
-- One-shot SQL copy of sessionevents.ACPEvent.SlimForStorage for rows written
-- before the rule existed; that method is the canonical definition.
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
