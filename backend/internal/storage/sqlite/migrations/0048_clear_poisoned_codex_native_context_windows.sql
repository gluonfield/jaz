-- +goose Up
UPDATE threads
SET context_window_tokens = 0
WHERE runtime = 'acp'
  AND acp_agent = 'codex'
  AND model_provider IN ('', 'codex', 'openai', 'openai-api-key')
  AND context_window_tokens IN (353400, 380000, 997500)
  AND model IN (
    'gpt-5.6-sol', 'openai/gpt-5.6-sol',
    'gpt-5.6-terra', 'openai/gpt-5.6-terra',
    'gpt-5.6-luna', 'openai/gpt-5.6-luna',
    'gpt-5.5', 'openai/gpt-5.5',
    'gpt-5.4', 'openai/gpt-5.4',
    'gpt-5.4-mini', 'openai/gpt-5.4-mini',
    'gpt-5.3-codex-spark', 'openai/gpt-5.3-codex-spark'
  );

UPDATE usage_events
SET context_window_tokens = 0
WHERE runtime = 'acp'
  AND agent = 'codex'
  AND model_provider IN ('', 'codex', 'openai', 'openai-api-key')
  AND context_window_tokens IN (353400, 380000, 997500)
  AND model IN (
    'gpt-5.6-sol', 'openai/gpt-5.6-sol',
    'gpt-5.6-terra', 'openai/gpt-5.6-terra',
    'gpt-5.6-luna', 'openai/gpt-5.6-luna',
    'gpt-5.5', 'openai/gpt-5.5',
    'gpt-5.4', 'openai/gpt-5.4',
    'gpt-5.4-mini', 'openai/gpt-5.4-mini',
    'gpt-5.3-codex-spark', 'openai/gpt-5.3-codex-spark'
  );

-- +goose Down
SELECT 1;
