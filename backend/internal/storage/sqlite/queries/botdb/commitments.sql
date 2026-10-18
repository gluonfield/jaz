-- name: InsertCommitment :one
INSERT INTO bot_commitments (bot_id, requester, request, note, opened_at_ms)
VALUES (sqlc.arg(bot_id), sqlc.arg(requester), sqlc.arg(request), sqlc.arg(note), sqlc.arg(opened_at_ms))
RETURNING id;

-- name: UpdateCommitment :execrows
UPDATE bot_commitments
SET request = sqlc.arg(request), note = sqlc.arg(note)
WHERE id = sqlc.arg(id) AND bot_id = sqlc.arg(bot_id) AND status = 'open';

-- name: CommitmentWorking :one
SELECT EXISTS (
  SELECT 1 FROM bot_commitment_thread_states WHERE commitment_id = sqlc.arg(id) AND state = 'working'
);

-- name: CommitmentStaged :one
-- A running turn of its bot holds an answer that delivers it.
SELECT EXISTS (
  SELECT 1 FROM bot_commitments c
  JOIN threads t ON t.id = c.bot_id
  JOIN json_each(COALESCE(json_extract(NULLIF(t.turn, ''), '$.output.done'), '[]')) d
  WHERE c.id = sqlc.arg(id) AND d.value = c.id
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
SELECT commitment_id, thread_id, slug, title, state FROM bot_commitment_thread_states
WHERE commitment_id IN (sqlc.slice(ids))
ORDER BY thread_id;
