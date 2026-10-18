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
  memory_path,
  model_provider,
  model,
  bot_id,
  event_trigger,
  webhook_hash,
  when_missed,
  thread
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
  sqlc.arg(memory_path),
  sqlc.arg(model_provider),
  sqlc.arg(model),
  sqlc.arg(bot_id),
  sqlc.arg(event_trigger),
  sqlc.arg(webhook_hash),
  sqlc.arg(when_missed),
  sqlc.arg(thread)
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
  model_provider = excluded.model_provider,
  model = excluded.model,
  bot_id = excluded.bot_id,
  event_trigger = excluded.event_trigger,
  webhook_hash = excluded.webhook_hash,
  when_missed = excluded.when_missed,
  thread = excluded.thread,
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
  memory_path,
  model_provider,
  model,
  bot_id,
  event_trigger,
  webhook_hash,
  when_missed,
  thread
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
  memory_path,
  model_provider,
  model,
  bot_id,
  event_trigger,
  webhook_hash,
  when_missed,
  thread
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
  created_at_ms,
  reason,
  delivered
) VALUES (
  sqlc.arg(id),
  sqlc.arg(loop_id),
  sqlc.narg(thread_id),
  sqlc.arg(scheduled_for_ms),
  sqlc.arg(started_at_ms),
  sqlc.arg(finished_at_ms),
  sqlc.arg(status),
  sqlc.narg(error),
  sqlc.arg(created_at_ms),
  sqlc.arg(reason),
  sqlc.arg(delivered)
)
ON CONFLICT(id) DO UPDATE SET
  loop_id = excluded.loop_id,
  thread_id = excluded.thread_id,
  scheduled_for_ms = excluded.scheduled_for_ms,
  started_at_ms = excluded.started_at_ms,
  finished_at_ms = excluded.finished_at_ms,
  status = excluded.status,
  error = excluded.error,
  created_at_ms = excluded.created_at_ms,
  reason = excluded.reason,
  delivered = excluded.delivered;

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
  created_at_ms,
  reason,
  delivered
FROM loop_runs
WHERE id = sqlc.arg(id)
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
  created_at_ms,
  reason,
  delivered
FROM loop_runs
WHERE loop_id = sqlc.arg(loop_id)
ORDER BY created_at_ms DESC
LIMIT sqlc.arg(limit);

-- name: GetRunBefore :one
SELECT
  r.id,
  r.loop_id,
  r.thread_id,
  r.scheduled_for_ms,
  r.started_at_ms,
  r.finished_at_ms,
  r.status,
  r.error,
  r.created_at_ms,
  r.reason,
  r.delivered
FROM loop_runs r, loop_runs this
WHERE this.id = sqlc.arg(id) AND r.loop_id = this.loop_id
  AND (r.created_at_ms < this.created_at_ms OR r.created_at_ms = this.created_at_ms AND r.rowid < this.rowid)
ORDER BY r.created_at_ms DESC, r.rowid DESC
LIMIT 1;

-- name: ListActiveRunsForLoop :many
SELECT
  id,
  loop_id,
  thread_id,
  scheduled_for_ms,
  started_at_ms,
  finished_at_ms,
  status,
  error,
  created_at_ms,
  reason,
  delivered
FROM loop_runs
WHERE loop_id = sqlc.arg(loop_id)
  AND status IN (sqlc.arg(starting_status), sqlc.arg(running_status))
ORDER BY created_at_ms ASC;

-- name: GetLastFinishedAt :one
SELECT CAST(COALESCE(MAX(finished_at_ms), 0) AS INTEGER)
FROM loop_runs
WHERE loop_id = sqlc.arg(loop_id) AND status = sqlc.arg(status);

-- name: LoopEventSeen :one
SELECT EXISTS (
  SELECT 1 FROM loop_events WHERE loop_id = sqlc.arg(loop_id) AND event_id = sqlc.arg(event_id)
);

-- name: GetLatestLoopEventAt :one
SELECT CAST(COALESCE(MAX(at_ms), 0) AS INTEGER) FROM loop_events WHERE loop_id = sqlc.arg(loop_id);

-- name: InsertLoopEvent :exec
INSERT OR IGNORE INTO loop_events (loop_id, event_id, run_id, at_ms)
VALUES (sqlc.arg(loop_id), sqlc.arg(event_id), sqlc.arg(run_id), sqlc.arg(at_ms));

-- name: ListActiveRunIDs :many
SELECT id
FROM loop_runs
WHERE status IN (sqlc.arg(starting_status), sqlc.arg(running_status));
