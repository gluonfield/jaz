-- name: StartSessionTurn :exec
UPDATE threads
SET status = 'running',
    error = NULL,
    turn = sqlc.arg(turn),
    updated_at_ms = sqlc.arg(started_at_ms),
    last_attention_at_ms = sqlc.arg(started_at_ms)
WHERE id = sqlc.arg(id);

-- name: ClaimTurn :execrows
UPDATE threads
SET status = 'running',
    error = NULL,
    turn = sqlc.arg(turn),
    updated_at_ms = sqlc.arg(started_at_ms),
    last_attention_at_ms = sqlc.arg(started_at_ms)
WHERE id = sqlc.arg(id) AND status <> 'running';

-- name: GetRunningTurn :one
SELECT turn FROM threads WHERE id = sqlc.arg(id) AND status = 'running' AND turn <> '';

-- name: SetRunningTurn :exec
UPDATE threads SET turn = sqlc.arg(turn) WHERE id = sqlc.arg(id) AND status = 'running' AND turn <> '';

-- name: SetTurnPromptState :execrows
UPDATE threads
SET turn = CASE WHEN json_extract(turn, '$.prompt.state') = 'confirmed' THEN turn
      ELSE json_set(turn, '$.prompt.state', CAST(sqlc.arg(state) AS TEXT)) END,
    acp_session_id = sqlc.narg(agent_session_id)
WHERE id = sqlc.arg(id) AND status = 'running' AND turn <> ''
  AND json_type(turn, '$.prompt') = 'object'
  AND json_extract(turn, '$.started_at') = sqlc.arg(started_at)
  AND (acp_session_id IS NULL OR acp_session_id = '' OR acp_session_id = sqlc.narg(agent_session_id));

-- name: SetTurnIntent :execrows
UPDATE threads
SET turn = json_set(turn,
    '$.goal_requested', json(CASE WHEN CAST(sqlc.arg(goal_requested) AS INTEGER) THEN 'true' ELSE 'false' END),
    '$.parent_visible', json(CASE WHEN CAST(sqlc.arg(parent_visible) AS INTEGER) THEN 'true' ELSE 'false' END),
    '$.notify_parent', json(CASE WHEN CAST(sqlc.arg(notify_parent) AS INTEGER) THEN 'true' ELSE 'false' END))
WHERE id = sqlc.arg(id) AND status = 'running' AND turn <> '';

-- name: AppendTurnSteer :exec
UPDATE threads
SET turn = json_set(turn, '$.steers', json_insert(COALESCE(json_extract(turn, '$.steers'), '[]'), '$[#]', json(sqlc.arg(steer))))
WHERE id = sqlc.arg(id) AND status = 'running' AND turn <> '';

-- name: TakeTurnSteer :exec
UPDATE threads
SET turn = json_remove(turn, '$.steers[0]')
WHERE id = sqlc.arg(id) AND status = 'running' AND json_array_length(NULLIF(turn, ''), '$.steers') > 0;

-- name: SetTurnRetries :execrows
UPDATE threads
SET turn = json_set(turn, '$.retries', CAST(sqlc.arg(retries) AS INTEGER))
WHERE id = sqlc.arg(id) AND status = 'running' AND turn <> '';

-- name: AppendTurnReply :execrows
UPDATE threads
SET turn = json_set(turn,
    '$.output.replies', json_insert(COALESCE(json_extract(turn, '$.output.replies'), '[]'), '$[#]', sqlc.arg(message)),
    '$.output.done', json((SELECT json_group_array(value) FROM (
        SELECT value FROM json_each(COALESCE(json_extract(turn, '$.output.done'), '[]'))
        UNION ALL SELECT value FROM json_each(sqlc.arg(done))))))
WHERE id = sqlc.arg(id) AND status = 'running'
  AND json_extract(NULLIF(turn, ''), '$.output.reply_to') IS NOT NULL;

-- name: MarkTurnShown :exec
UPDATE threads
SET turn = json_set(turn, '$.output.shown', json('true'))
WHERE id = sqlc.arg(id) AND status = 'running'
  AND json_type(NULLIF(turn, ''), '$.output') = 'object';

-- name: ResetTurnHops :exec
UPDATE threads
SET turn = json_remove(turn, '$.output.hops')
WHERE id = sqlc.arg(id) AND status = 'running' AND turn <> '';

-- name: FinishSessionTurn :execrows
-- A compact brings the user nothing new to read. What a turn's agent was shown
-- is kept only once the agent took the turn's prompt.
UPDATE threads
SET status = sqlc.arg(status), turn = '', error = sqlc.narg(error), turns_ended = turns_ended + 1,
    unread = CASE WHEN sqlc.arg(status) = 'idle' AND json_extract(turn, '$.active_operation') IS NOT 'compact' THEN 1 ELSE unread END,
    updated_at_ms = sqlc.arg(finished_at_ms),
    last_attention_at_ms = sqlc.arg(finished_at_ms),
    last_completed_at_ms = CASE WHEN sqlc.arg(status) = 'idle' THEN sqlc.arg(finished_at_ms) ELSE last_completed_at_ms END,
    completed_turn_started_at_ms = CASE WHEN sqlc.arg(status) = 'idle'
      THEN COALESCE(CAST(unixepoch(json_extract(turn, '$.started_at'), 'subsec') * 1000 AS INTEGER), completed_turn_started_at_ms)
      ELSE completed_turn_started_at_ms END,
    prompt_digest = CASE WHEN sqlc.arg(status) <> 'idle' THEN prompt_digest
      WHEN json_extract(turn, '$.active_operation') IS 'compact' THEN ''
      WHEN json_extract(turn, '$.prompt.state') IS NOT 'confirmed' THEN prompt_digest
      ELSE COALESCE(json_extract(turn, '$.prompt_digest'), prompt_digest) END
WHERE id = sqlc.arg(id) AND turn <> '';

-- name: AppendQueuedTurn :execrows
UPDATE threads
SET queued_messages = json_insert(COALESCE(NULLIF(queued_messages, ''), '[]'), '$[#]', json(sqlc.arg(message)))
WHERE id = sqlc.arg(id) AND archived = 0;

-- name: ClaimQueuedTurn :execrows
UPDATE threads
SET status = 'running', error = NULL, turn = sqlc.arg(turn),
    queued_messages = sqlc.arg(queued_messages), title = sqlc.narg(title),
    updated_at_ms = sqlc.arg(started_at_ms), last_attention_at_ms = sqlc.arg(started_at_ms)
WHERE id = sqlc.arg(id) AND status <> 'running';
