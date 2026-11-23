-- name: InsertUsageEvent :exec
INSERT INTO usage_events (
  thread_id,
  workspace_id,
  runtime,
  agent,
  model_provider,
  model,
  input_tokens,
  cached_input_tokens,
  cached_write_tokens,
  output_tokens,
  reasoning_output_tokens,
  total_tokens,
  context_tokens,
  context_window_tokens,
  source,
  source_type,
  created_at_ms
) VALUES (
  sqlc.arg(thread_id),
  sqlc.arg(workspace_id),
  sqlc.arg(runtime),
  sqlc.arg(agent),
  sqlc.arg(model_provider),
  sqlc.arg(model),
  sqlc.arg(input_tokens),
  sqlc.arg(cached_input_tokens),
  sqlc.arg(cached_write_tokens),
  sqlc.arg(output_tokens),
  sqlc.arg(reasoning_output_tokens),
  sqlc.arg(total_tokens),
  sqlc.arg(context_tokens),
  sqlc.arg(context_window_tokens),
  sqlc.arg(source),
  sqlc.arg(source_type),
  sqlc.arg(created_at_ms)
);

-- name: ListUsageEvents :many
SELECT
  thread_id,
  runtime,
  agent,
  model_provider,
  model,
  input_tokens,
  cached_input_tokens,
  cached_write_tokens,
  output_tokens,
  reasoning_output_tokens,
  total_tokens,
  source,
  source_type,
  created_at_ms
FROM (
  SELECT
    u.thread_id,
    u.runtime,
    u.agent,
    u.model_provider,
    u.model,
    u.input_tokens,
    u.cached_input_tokens,
    u.cached_write_tokens,
    u.output_tokens,
    u.reasoning_output_tokens,
    u.total_tokens,
    u.source,
    u.source_type,
    u.created_at_ms
  FROM usage_events u
  WHERE u.created_at_ms >= sqlc.arg(since)
    AND u.created_at_ms < sqlc.arg(until)
    AND (sqlc.arg(workspace_id) = '' OR u.workspace_id = sqlc.arg(workspace_id))
  UNION ALL
  SELECT
    c.thread_id,
    'acp' AS runtime,
    'codex' AS agent,
    c.model_profile_id AS model_provider,
    c.model,
    c.input_tokens,
    c.cached_input_tokens,
    0 AS cached_write_tokens,
    c.output_tokens,
    0 AS reasoning_output_tokens,
    c.input_tokens + c.output_tokens AS total_tokens,
    'turn' AS source,
    '' AS source_type,
    c.created_at_ms
  FROM cloud_usage_events c
  WHERE c.created_at_ms >= sqlc.arg(since)
    AND c.created_at_ms < sqlc.arg(until)
    AND sqlc.arg(workspace_id) != ''
    AND sqlc.arg(workspace_id) != 'local'
    AND c.account_id = sqlc.arg(workspace_id)
)
ORDER BY created_at_ms;
