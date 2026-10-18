-- name: StartSessionTurn :exec
UPDATE threads
SET status = 'running',
    error = NULL,
    turn = sqlc.arg(turn),
    updated_at_ms = sqlc.arg(started_at_ms),
    last_attention_at_ms = sqlc.arg(started_at_ms)
WHERE id = sqlc.arg(id);

-- name: SetTurnIntent :execrows
UPDATE threads
SET turn = json_set(turn,
    '$.goal_requested', json(CASE WHEN CAST(sqlc.arg(goal_requested) AS INTEGER) THEN 'true' ELSE 'false' END),
    '$.parent_visible', json(CASE WHEN CAST(sqlc.arg(parent_visible) AS INTEGER) THEN 'true' ELSE 'false' END),
    '$.notify_parent', json(CASE WHEN CAST(sqlc.arg(notify_parent) AS INTEGER) THEN 'true' ELSE 'false' END))
WHERE id = sqlc.arg(id) AND status = 'running' AND turn <> '';

-- name: AppendTurnReply :execrows
UPDATE threads
SET turn = json_set(turn, '$.output.replies', json_insert(COALESCE(json_extract(turn, '$.output.replies'), '[]'), '$[#]', sqlc.arg(message)))
WHERE id = sqlc.arg(id) AND status = 'running'
  AND json_extract(NULLIF(turn, ''), '$.output.reply_to') IS NOT NULL;

-- name: FinishSessionTurn :execrows
-- A compact brings the user nothing new to read.
UPDATE threads
SET status = sqlc.arg(status), turn = '', error = sqlc.narg(error),
    unread = CASE WHEN sqlc.arg(status) = 'idle' AND json_extract(turn, '$.active_operation') IS NOT 'compact' THEN 1 ELSE unread END,
    updated_at_ms = sqlc.arg(finished_at_ms),
    last_attention_at_ms = sqlc.arg(finished_at_ms),
    last_completed_at_ms = CASE WHEN sqlc.arg(status) = 'idle' THEN sqlc.arg(finished_at_ms) ELSE last_completed_at_ms END
WHERE id = sqlc.arg(id) AND turn <> '';

-- name: AppendQueuedTurn :execrows
UPDATE threads
SET queued_messages = json_insert(COALESCE(NULLIF(queued_messages, ''), '[]'), '$[#]', json(sqlc.arg(message)))
WHERE id = sqlc.arg(id) AND archived = 0;

-- name: ClaimQueuedTurn :exec
UPDATE threads
SET status = 'running', error = NULL, turn = sqlc.arg(turn),
    queued_messages = sqlc.arg(queued_messages), title = sqlc.narg(title),
    updated_at_ms = sqlc.arg(started_at_ms), last_attention_at_ms = sqlc.arg(started_at_ms)
WHERE id = sqlc.arg(id);
