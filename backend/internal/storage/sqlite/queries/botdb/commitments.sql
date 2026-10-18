-- name: InsertCommitment :one
INSERT INTO bot_commitments (bot_id, requester, request, note, opened_at_ms)
VALUES (sqlc.arg(bot_id), sqlc.arg(requester), sqlc.arg(request), sqlc.arg(note), sqlc.arg(opened_at_ms))
RETURNING id;

-- name: UpdateCommitment :execrows
UPDATE bot_commitments
SET request = sqlc.arg(request), note = sqlc.arg(note)
WHERE id = sqlc.arg(id) AND bot_id = sqlc.arg(bot_id) AND status = 'open';

-- name: CommitmentWorking :one
-- A thread works while it runs, waits to resume or has turns queued.
SELECT EXISTS (
  SELECT 1 FROM bot_commitment_threads l JOIN threads t ON t.id = l.thread_id
  WHERE l.commitment_id = sqlc.arg(id) AND t.archived = 0
    AND (t.status IN ('running', 'interrupted') OR json_array_length(COALESCE(NULLIF(t.queued_messages, ''), '[]')) > 0)
);

-- name: CloseCommitment :execrows
UPDATE bot_commitments
SET status = sqlc.arg(status), note = COALESCE(sqlc.narg(note), note), closed_at_ms = sqlc.arg(closed_at_ms)
WHERE id = sqlc.arg(id) AND status = 'open';

-- name: ListOpenCommitments :many
SELECT * FROM bot_commitments WHERE bot_id = sqlc.arg(bot_id) AND status = 'open' ORDER BY id;

-- name: ListClosedCommitments :many
SELECT * FROM bot_commitments
WHERE bot_id = sqlc.arg(bot_id) AND status <> 'open' AND closed_at_ms >= sqlc.arg(since_ms)
ORDER BY id;

-- name: GetCommitment :one
SELECT * FROM bot_commitments WHERE id = ?;

-- name: LinkCommitmentThread :execrows
INSERT INTO bot_commitment_threads (thread_id, commitment_id)
SELECT sqlc.arg(thread_id), id FROM bot_commitments
WHERE id = sqlc.arg(commitment_id) AND bot_id = sqlc.arg(bot_id) AND status = 'open';

-- name: ListCommitmentThreads :many
SELECT commitment_id, thread_id FROM bot_commitment_threads
WHERE commitment_id IN (sqlc.slice(ids))
ORDER BY rowid;
