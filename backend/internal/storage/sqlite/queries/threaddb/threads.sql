-- name: ListSessions :many
SELECT
  id,
  slug,
  title,
  parent_id,
  status,
  error,
  runtime,
  acp_agent,
  acp_session_id,
  cwd,
  model_provider,
  model,
  reasoning_effort,
  input_tokens,
  cached_input_tokens,
  output_tokens,
  reasoning_output_tokens,
  total_tokens,
  queued_messages,
  source_type,
  source_id,
  archived,
  created_at_ms,
  updated_at_ms,
  context_tokens,
  context_window_tokens,
  cached_write_tokens,
  project_path,
  last_attention_at_ms,
  pinned,
  artifact_surface,
  mcp_server_policy,
  pending_steer_message,
  unread,
  goal,
  manual_title,
  last_completed_at_ms
FROM threads;

-- name: GetSession :one
SELECT
  id,
  slug,
  title,
  parent_id,
  status,
  error,
  runtime,
  acp_agent,
  acp_session_id,
  cwd,
  model_provider,
  model,
  reasoning_effort,
  input_tokens,
  cached_input_tokens,
  output_tokens,
  reasoning_output_tokens,
  total_tokens,
  queued_messages,
  source_type,
  source_id,
  archived,
  created_at_ms,
  updated_at_ms,
  context_tokens,
  context_window_tokens,
  cached_write_tokens,
  project_path,
  last_attention_at_ms,
  pinned,
  artifact_surface,
  mcp_server_policy,
  pending_steer_message,
  unread,
  goal,
  manual_title,
  last_completed_at_ms
FROM threads
WHERE id = sqlc.arg(ref) OR slug = sqlc.arg(ref)
LIMIT 1;

-- name: GetThreadIDByID :one
SELECT id
FROM threads
WHERE id = sqlc.arg(id)
LIMIT 1;

-- name: GetThreadIDBySlug :one
SELECT id
FROM threads
WHERE slug = sqlc.arg(slug)
LIMIT 1;

-- name: UpsertSession :exec
INSERT INTO threads (
  id,
  slug,
  title,
  manual_title,
  parent_id,
  status,
  runtime,
  acp_agent,
  acp_session_id,
  cwd,
  artifact_surface,
  mcp_server_policy,
  project_path,
  error,
  model_provider,
  model,
  reasoning_effort,
  input_tokens,
  cached_input_tokens,
  cached_write_tokens,
  output_tokens,
  reasoning_output_tokens,
  total_tokens,
  context_tokens,
  context_window_tokens,
  queued_messages,
  source_type,
  source_id,
  archived,
  created_at_ms,
  updated_at_ms,
  last_attention_at_ms,
  pinned,
  pending_steer_message,
  unread,
  goal
) VALUES (
  sqlc.arg(id),
  sqlc.arg(slug),
  sqlc.narg(title),
  sqlc.arg(manual_title),
  sqlc.narg(parent_id),
  sqlc.arg(status),
  sqlc.arg(runtime),
  sqlc.narg(acp_agent),
  sqlc.narg(acp_session_id),
  sqlc.narg(cwd),
  sqlc.narg(artifact_surface),
  sqlc.narg(mcp_server_policy),
  sqlc.narg(project_path),
  sqlc.narg(error),
  sqlc.narg(model_provider),
  sqlc.narg(model),
  sqlc.narg(reasoning_effort),
  sqlc.arg(input_tokens),
  sqlc.arg(cached_input_tokens),
  sqlc.arg(cached_write_tokens),
  sqlc.arg(output_tokens),
  sqlc.arg(reasoning_output_tokens),
  sqlc.arg(total_tokens),
  sqlc.arg(context_tokens),
  sqlc.arg(context_window_tokens),
  sqlc.arg(queued_messages),
  sqlc.narg(source_type),
  sqlc.narg(source_id),
  sqlc.arg(archived),
  sqlc.arg(created_at_ms),
  sqlc.arg(updated_at_ms),
  sqlc.arg(last_attention_at_ms),
  sqlc.arg(pinned),
  sqlc.arg(pending_steer_message),
  sqlc.arg(unread),
  sqlc.arg(goal)
)
ON CONFLICT(id) DO UPDATE SET
  slug = excluded.slug,
  title = excluded.title,
  manual_title = excluded.manual_title,
  parent_id = excluded.parent_id,
  status = excluded.status,
  error = excluded.error,
  runtime = excluded.runtime,
  acp_agent = excluded.acp_agent,
  acp_session_id = excluded.acp_session_id,
  cwd = excluded.cwd,
  artifact_surface = excluded.artifact_surface,
  mcp_server_policy = excluded.mcp_server_policy,
  project_path = excluded.project_path,
  model_provider = excluded.model_provider,
  model = excluded.model,
  reasoning_effort = excluded.reasoning_effort,
  input_tokens = excluded.input_tokens,
  cached_input_tokens = excluded.cached_input_tokens,
  cached_write_tokens = excluded.cached_write_tokens,
  output_tokens = excluded.output_tokens,
  reasoning_output_tokens = excluded.reasoning_output_tokens,
  total_tokens = excluded.total_tokens,
  context_tokens = excluded.context_tokens,
  context_window_tokens = excluded.context_window_tokens,
  queued_messages = excluded.queued_messages,
  source_type = excluded.source_type,
  source_id = excluded.source_id,
  archived = excluded.archived,
  created_at_ms = excluded.created_at_ms,
  updated_at_ms = excluded.updated_at_ms,
  last_attention_at_ms = excluded.last_attention_at_ms,
  pinned = excluded.pinned,
  pending_steer_message = excluded.pending_steer_message,
  unread = excluded.unread,
  goal = excluded.goal;

-- name: ListSessionSubtree :many
WITH RECURSIVE subtree(id) AS (
  SELECT threads.id FROM threads WHERE threads.id = sqlc.arg(id)
  UNION
  SELECT threads.id
  FROM threads
  JOIN subtree ON threads.parent_id = subtree.id
)
SELECT subtree.id FROM subtree;

-- name: SetArchived :exec
UPDATE threads
SET
  archived = sqlc.arg(archived),
  unread = CASE WHEN sqlc.arg(archived) != 0 THEN 0 ELSE unread END
WHERE id IN (sqlc.slice('ids'));

-- name: SetPinned :exec
UPDATE threads
SET pinned = sqlc.arg(pinned)
WHERE id = sqlc.arg(id) OR parent_id = sqlc.arg(id);

-- name: UpdateSessionTitle :exec
UPDATE threads
SET
  title = sqlc.narg(title),
  manual_title = 1
WHERE id = sqlc.arg(id);

-- name: UpdateSessionTitleFromRuntime :execrows
UPDATE threads
SET
  title = sqlc.narg(title),
  manual_title = 0
WHERE id = sqlc.arg(id) AND manual_title = 0;

-- name: UpdateSessionStatus :exec
UPDATE threads
SET
  status = sqlc.arg(status),
  error = sqlc.narg(error),
  updated_at_ms = sqlc.arg(updated_at_ms),
  last_attention_at_ms = CASE
    WHEN CAST(sqlc.arg(touch_attention) AS INTEGER) != 0 THEN sqlc.arg(last_attention_at_ms)
    ELSE last_attention_at_ms
  END
WHERE id = sqlc.arg(id);

-- name: CompleteSession :exec
UPDATE threads
SET
  status = 'idle',
  error = NULL,
  unread = 1,
  updated_at_ms = sqlc.arg(completed_at_ms),
  last_attention_at_ms = sqlc.arg(completed_at_ms),
  last_completed_at_ms = sqlc.arg(completed_at_ms)
WHERE id = sqlc.arg(id);

-- name: TouchThread :exec
UPDATE threads
SET updated_at_ms = sqlc.arg(updated_at_ms)
WHERE id = sqlc.arg(id);

-- name: TouchSessionAttention :exec
UPDATE threads
SET
  updated_at_ms = sqlc.arg(updated_at_ms),
  last_attention_at_ms = sqlc.arg(last_attention_at_ms)
WHERE id = sqlc.arg(id);

-- name: SetThreadUnread :exec
UPDATE threads
SET unread = sqlc.arg(unread)
WHERE id = sqlc.arg(id);

-- name: UpdateGoal :exec
UPDATE threads
SET
  goal = sqlc.arg(goal),
  updated_at_ms = sqlc.arg(updated_at_ms)
WHERE id = sqlc.arg(id);

-- name: UpdateACPState :exec
UPDATE threads
SET
  status = sqlc.arg(status),
  error = sqlc.narg(error),
  updated_at_ms = sqlc.arg(updated_at_ms)
WHERE id = sqlc.arg(id);

-- name: AddUsage :exec
UPDATE threads SET
  input_tokens = input_tokens + sqlc.arg(input_tokens),
  cached_input_tokens = cached_input_tokens + sqlc.arg(cached_input_tokens),
  cached_write_tokens = cached_write_tokens + sqlc.arg(cached_write_tokens),
  output_tokens = output_tokens + sqlc.arg(output_tokens),
  reasoning_output_tokens = reasoning_output_tokens + sqlc.arg(reasoning_output_tokens),
  total_tokens = total_tokens + sqlc.arg(total_tokens),
  context_tokens = CASE
    WHEN CAST(sqlc.arg(context_tokens) AS INTEGER) > 0 THEN CAST(sqlc.arg(context_tokens) AS INTEGER)
    ELSE context_tokens
  END,
  context_window_tokens = CASE
    WHEN CAST(sqlc.arg(context_window_tokens) AS INTEGER) > 0 THEN CAST(sqlc.arg(context_window_tokens) AS INTEGER)
    ELSE context_window_tokens
  END,
  updated_at_ms = sqlc.arg(updated_at_ms)
WHERE id = sqlc.arg(id);

-- name: ResetRunningThreads :exec
UPDATE threads
SET
  status = sqlc.arg(status),
  error = sqlc.narg(error),
  pending_steer_message = '',
  updated_at_ms = sqlc.arg(updated_at_ms)
WHERE status = sqlc.arg(running_status);

-- name: ListErrorThreadIDsWithoutError :many
SELECT id
FROM threads
WHERE status = sqlc.arg(status)
  AND (error IS NULL OR error = '');

-- name: SetThreadError :exec
UPDATE threads
SET error = sqlc.narg(error)
WHERE id = sqlc.arg(id);
