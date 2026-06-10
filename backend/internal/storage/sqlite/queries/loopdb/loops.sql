-- name: UpsertLoop :exec
INSERT INTO loops (
  id,
  name,
  prompt,
  schedule_kind,
  schedule_expr,
  timezone,
  status,
  runtime,
  acp_agent,
  next_run_at_ms,
  last_run_at_ms,
  last_run_id,
  last_run_thread_id,
  last_run_status,
  last_error,
  created_at_ms,
  updated_at_ms,
  reasoning_effort,
  directory,
  memory_path
) VALUES (
  sqlc.arg(id),
  sqlc.arg(name),
  sqlc.arg(prompt),
  sqlc.arg(schedule_kind),
  sqlc.arg(schedule_expr),
  sqlc.arg(timezone),
  sqlc.arg(status),
  sqlc.arg(runtime),
  sqlc.narg(acp_agent),
  sqlc.arg(next_run_at_ms),
  sqlc.arg(last_run_at_ms),
  sqlc.narg(last_run_id),
  sqlc.narg(last_run_thread_id),
  sqlc.narg(last_run_status),
  sqlc.narg(last_error),
  sqlc.arg(created_at_ms),
  sqlc.arg(updated_at_ms),
  sqlc.arg(reasoning_effort),
  sqlc.arg(directory),
  sqlc.arg(memory_path)
)
ON CONFLICT(id) DO UPDATE SET
  name = excluded.name,
  prompt = excluded.prompt,
  schedule_kind = excluded.schedule_kind,
  schedule_expr = excluded.schedule_expr,
  timezone = excluded.timezone,
  status = excluded.status,
  runtime = excluded.runtime,
  acp_agent = excluded.acp_agent,
  reasoning_effort = excluded.reasoning_effort,
  directory = excluded.directory,
  memory_path = excluded.memory_path,
  next_run_at_ms = excluded.next_run_at_ms,
  last_run_at_ms = excluded.last_run_at_ms,
  last_run_id = excluded.last_run_id,
  last_run_thread_id = excluded.last_run_thread_id,
  last_run_status = excluded.last_run_status,
  last_error = excluded.last_error,
  created_at_ms = excluded.created_at_ms,
  updated_at_ms = excluded.updated_at_ms;

-- name: GetLoop :one
SELECT
  id,
  name,
  prompt,
  schedule_kind,
  schedule_expr,
  timezone,
  status,
  runtime,
  acp_agent,
  next_run_at_ms,
  last_run_at_ms,
  last_run_id,
  last_run_thread_id,
  last_run_status,
  last_error,
  created_at_ms,
  updated_at_ms,
  reasoning_effort,
  directory,
  memory_path
FROM loops
WHERE id = sqlc.arg(id)
LIMIT 1;

-- name: ListLoops :many
SELECT
  id,
  name,
  prompt,
  schedule_kind,
  schedule_expr,
  timezone,
  status,
  runtime,
  acp_agent,
  next_run_at_ms,
  last_run_at_ms,
  last_run_id,
  last_run_thread_id,
  last_run_status,
  last_error,
  created_at_ms,
  updated_at_ms,
  reasoning_effort,
  directory,
  memory_path
FROM loops
WHERE status <> sqlc.arg(deleted_status)
ORDER BY updated_at_ms DESC;

-- name: ListDueLoopIDs :many
SELECT id
FROM loops
WHERE status = sqlc.arg(active_status)
  AND next_run_at_ms > 0
  AND next_run_at_ms <= sqlc.arg(now_ms)
ORDER BY next_run_at_ms ASC;

-- name: UpsertRun :exec
INSERT INTO loop_runs (
  id,
  loop_id,
  thread_id,
  scheduled_for_ms,
  started_at_ms,
  finished_at_ms,
  status,
  error,
  created_at_ms
) VALUES (
  sqlc.arg(id),
  sqlc.arg(loop_id),
  sqlc.narg(thread_id),
  sqlc.arg(scheduled_for_ms),
  sqlc.arg(started_at_ms),
  sqlc.arg(finished_at_ms),
  sqlc.arg(status),
  sqlc.narg(error),
  sqlc.arg(created_at_ms)
)
ON CONFLICT(id) DO UPDATE SET
  loop_id = excluded.loop_id,
  thread_id = excluded.thread_id,
  scheduled_for_ms = excluded.scheduled_for_ms,
  started_at_ms = excluded.started_at_ms,
  finished_at_ms = excluded.finished_at_ms,
  status = excluded.status,
  error = excluded.error,
  created_at_ms = excluded.created_at_ms;

-- name: GetRun :one
SELECT
  id,
  loop_id,
  thread_id,
  scheduled_for_ms,
  started_at_ms,
  finished_at_ms,
  status,
  error,
  created_at_ms
FROM loop_runs
WHERE id = sqlc.arg(id)
LIMIT 1;

-- name: GetLatestRunByThread :one
SELECT
  id,
  loop_id,
  thread_id,
  scheduled_for_ms,
  started_at_ms,
  finished_at_ms,
  status,
  error,
  created_at_ms
FROM loop_runs
WHERE thread_id = sqlc.arg(thread_id)
ORDER BY created_at_ms DESC
LIMIT 1;

-- name: ListRunsByLoop :many
SELECT
  id,
  loop_id,
  thread_id,
  scheduled_for_ms,
  started_at_ms,
  finished_at_ms,
  status,
  error,
  created_at_ms
FROM loop_runs
WHERE loop_id = sqlc.arg(loop_id)
ORDER BY created_at_ms DESC
LIMIT sqlc.arg(limit);

-- name: GetActiveRunIDForLoop :one
SELECT id
FROM loop_runs
WHERE loop_id = sqlc.arg(loop_id)
  AND status IN (sqlc.arg(starting_status), sqlc.arg(running_status))
LIMIT 1;

-- name: ListActiveRunIDs :many
SELECT id
FROM loop_runs
WHERE status IN (sqlc.arg(starting_status), sqlc.arg(running_status));
