-- +goose Up
UPDATE threads
SET model_provider = 'openai'
WHERE runtime = 'acp'
  AND acp_agent = 'codex'
  AND model_provider = 'codex';

UPDATE usage_events
SET model_provider = 'openai'
WHERE runtime = 'acp'
  AND agent = 'codex'
  AND model_provider = 'codex';

-- +goose Down
SELECT 1;
