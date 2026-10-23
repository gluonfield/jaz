-- name: InsertCommitment :one
INSERT INTO bot_commitments (bot_id, requester, request, note, status, opened_at_ms)
VALUES (sqlc.arg(bot_id), sqlc.arg(requester), sqlc.arg(request), sqlc.arg(note), 'pending', sqlc.arg(opened_at_ms))
RETURNING id;

-- name: UpdateCommitment :execrows
UPDATE bot_commitments
SET request = coalesce(sqlc.narg(request), request), note = coalesce(sqlc.narg(note), note), status = coalesce(sqlc.narg(status), status),
  question_seq = CASE WHEN sqlc.narg(status) IS NULL OR sqlc.narg(status) = 'waiting_for_user' THEN question_seq ELSE 0 END
WHERE id = sqlc.arg(id) AND bot_id = sqlc.arg(bot_id) AND status IN ('pending', 'in_progress', 'waiting_for_user')
  AND (sqlc.narg(status) IS NOT 'waiting_for_user' OR (status = 'waiting_for_user' AND question_seq = sqlc.arg(question_seq)));

-- name: AskCommitment :exec
UPDATE bot_commitments SET status = 'waiting_for_user', question_seq = sqlc.arg(question_seq)
WHERE id = sqlc.arg(id) AND bot_id = sqlc.arg(bot_id) AND status IN ('pending', 'in_progress', 'waiting_for_user');

-- name: AnswerTaskQuestions :exec
UPDATE bot_commitments SET status = 'pending', question_seq = 0, wake_at_ms = 0, wake_attempts = 0, wake_progress = ''
WHERE bot_id = sqlc.arg(bot_id) AND status = 'waiting_for_user';

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
WHERE id = sqlc.arg(id) AND status IN ('pending', 'in_progress', 'waiting_for_user');

-- name: DropDeliveredResults :exec
-- The results a commitment's threads queued for its bot before it closed as
-- done: they had all finished, so the delivery answered them.
UPDATE threads
SET queued_messages = (
  SELECT json_group_array(json(kept.value)) FROM (
    SELECT m.value FROM json_each(threads.queued_messages) m
    WHERE json_extract(m.value, '$.kind') IS NOT 'internal'
       OR COALESCE(json_extract(m.value, '$.from'), '') NOT IN (
         SELECT thread_id FROM bot_commitment_threads WHERE commitment_id = sqlc.arg(id))
    ORDER BY m.key) kept)
WHERE id = (SELECT bot_id FROM bot_commitments WHERE id = sqlc.arg(id)) AND queued_messages <> '';

-- name: ListOpenCommitments :many
SELECT * FROM bot_commitments WHERE bot_id = sqlc.arg(bot_id) AND status IN ('pending', 'in_progress', 'waiting_for_user') ORDER BY id;

-- name: ListClosedCommitments :many
SELECT * FROM bot_commitments
WHERE bot_id = sqlc.arg(bot_id) AND status IN ('completed', 'cancelled') AND closed_at_ms >= sqlc.arg(since_ms)
ORDER BY id;

-- name: GetCommitment :one
SELECT * FROM bot_commitments WHERE id = ?;

-- name: LinkCommitmentThread :execrows
INSERT INTO bot_commitment_threads (thread_id, commitment_id)
SELECT sqlc.arg(thread_id), c.id FROM bot_commitments c
WHERE c.id = sqlc.arg(commitment_id) AND c.bot_id = sqlc.arg(bot_id) AND c.status IN ('pending', 'in_progress')
  AND NOT EXISTS (
    SELECT 1 FROM bot_commitment_dependencies d JOIN bot_commitments p ON p.id = d.prerequisite_id
    WHERE d.commitment_id = c.id AND p.status IN ('pending', 'in_progress', 'waiting_for_user')
  );

-- name: StartCommitment :exec
UPDATE bot_commitments SET status = 'in_progress' WHERE id = ?;

-- name: ListCommitmentMotion :many
SELECT c.id, CAST(p.moving AS INTEGER) AS moving, CAST(p.blocked AS INTEGER) AS blocked,
  CAST(c.status <> 'waiting_for_user' AND NOT p.moving AND NOT p.blocked AND c.wake_attempts > 0 AS INTEGER) AS stalled
FROM bot_commitments c
JOIN bot_commitment_progress p ON p.commitment_id = c.id
WHERE c.id IN (sqlc.slice(ids));

-- name: WakeStalled :many
UPDATE bot_commitments
SET wake_progress = (SELECT progress FROM bot_commitment_progress WHERE commitment_id = bot_commitments.id),
  wake_at_ms = CAST(sqlc.arg(now_ms) AS INTEGER) + CASE WHEN bot_commitments.wake_progress = (SELECT progress FROM bot_commitment_progress WHERE commitment_id = bot_commitments.id)
    THEN min(300000, 30000 << bot_commitments.wake_attempts) ELSE 30000 END,
  wake_attempts = CASE WHEN bot_commitments.wake_progress = (SELECT progress FROM bot_commitment_progress WHERE commitment_id = bot_commitments.id)
    THEN min(4, bot_commitments.wake_attempts + 1) ELSE 1 END
WHERE id IN (
  SELECT w.id FROM bot_commitment_watch w
  WHERE w.bot_id = sqlc.arg(bot_id) AND (w.wake_at_ms <= CAST(sqlc.arg(now_ms) AS INTEGER) OR w.wake_progress <> w.progress)
)
RETURNING id;

-- name: ListWatchBots :many
SELECT DISTINCT bot_id FROM bot_commitment_watch
WHERE wake_at_ms <= CAST(sqlc.arg(now_ms) AS INTEGER) OR wake_progress <> progress;

-- name: DeleteCommitmentDependencies :exec
DELETE FROM bot_commitment_dependencies WHERE commitment_id = ?;

-- name: LinkCommitmentDependency :execrows
INSERT INTO bot_commitment_dependencies (commitment_id, prerequisite_id)
SELECT c.id, p.id FROM bot_commitments c JOIN bot_commitments p ON p.bot_id = c.bot_id
WHERE c.id = sqlc.arg(commitment_id) AND p.id = sqlc.arg(prerequisite_id);

-- name: CommitmentDependencyCycle :one
WITH RECURSIVE ancestors(id) AS (
  SELECT id FROM bot_commitments WHERE id = sqlc.arg(prerequisite_id)
  UNION
  SELECT d.prerequisite_id FROM bot_commitment_dependencies d JOIN ancestors a ON a.id = d.commitment_id
)
SELECT EXISTS (SELECT 1 FROM ancestors WHERE id = sqlc.arg(commitment_id));

-- name: ListCommitmentDependencies :many
SELECT commitment_id, prerequisite_id FROM bot_commitment_dependencies
WHERE commitment_id IN (sqlc.slice(ids)) ORDER BY prerequisite_id;

-- name: ListCommitmentThreads :many
SELECT s.commitment_id, s.thread_id, s.slug, s.title, s.state FROM bot_commitment_thread_states s
JOIN bot_commitment_threads l ON l.id = s.linked AND l.thread_id = s.thread_id
WHERE s.commitment_id IN (sqlc.slice(ids))
ORDER BY s.linked;
