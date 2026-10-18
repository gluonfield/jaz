-- name: InsertCommitment :one
INSERT INTO bot_commitments (bot_id, requester, request, note, opened_at_ms)
VALUES (sqlc.arg(bot_id), sqlc.arg(requester), sqlc.arg(request), sqlc.arg(note), sqlc.arg(opened_at_ms))
RETURNING id;

-- name: UpdateCommitment :execrows
UPDATE bot_commitments
SET request = sqlc.arg(request), note = sqlc.arg(note)
WHERE id = sqlc.arg(id) AND bot_id = sqlc.arg(bot_id) AND status = 'open';

-- name: CloseCommitment :execrows
UPDATE bot_commitments
SET status = sqlc.arg(status), note = COALESCE(sqlc.narg(note), note), closed_at_ms = sqlc.arg(closed_at_ms)
WHERE id = sqlc.arg(id) AND status = 'open';

-- name: ListBotCommitments :many
SELECT * FROM bot_commitments
WHERE bot_id = sqlc.arg(bot_id) AND (status = 'open' OR closed_at_ms >= sqlc.arg(closed_since_ms))
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
