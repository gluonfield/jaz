-- name: StartSessionTurn :exec
UPDATE threads
SET status = 'running',
    error = NULL,
    turn = sqlc.arg(turn),
    updated_at_ms = sqlc.arg(started_at_ms),
    last_attention_at_ms = sqlc.arg(started_at_ms)
WHERE id = sqlc.arg(id);
