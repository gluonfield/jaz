-- name: InsertUsageEvent :exec
INSERT INTO usage_events (
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
  context_tokens,
  context_window_tokens,
  source,
  created_at_ms
) VALUES (
  sqlc.arg(thread_id),
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
  sqlc.arg(created_at_ms)
);

-- name: ListUsageEventsSince :many
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
  created_at_ms
FROM usage_events
WHERE created_at_ms >= sqlc.arg(created_at_ms)
ORDER BY created_at_ms;
