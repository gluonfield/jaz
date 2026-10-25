-- name: ListFinishedBotWork :many
WITH RECURSIVE finished(id) AS (
  SELECT threads.id FROM threads WHERE id IN (
    SELECT l.thread_id FROM bot_commitment_threads l
    JOIN bot_commitments c ON c.id = l.commitment_id
    WHERE c.status IN ('completed', 'cancelled') AND c.closed_at_ms < sqlc.arg(before_ms)
  )
  UNION
  SELECT threads.id FROM threads JOIN finished ON threads.parent_id = finished.id
  WHERE threads.source_type = 'bot_worker'
)
SELECT finished.id FROM finished;

-- name: ArchiveFinishedBotWork :exec
UPDATE threads SET archived = 1, unread = 0
WHERE threads.id = sqlc.arg(id)
  AND substr(cwd, 1, length(sqlc.arg(worktree_root)) + 1) = sqlc.arg(worktree_root) || '/'
  AND source_type = 'bot_worker' AND archived = 0 AND pinned = 0
  AND status = 'idle' AND turn = '' AND background_work = 0
  AND queued_messages IN ('', '[]') AND pending_steer_message = ''
  AND last_completed_at_ms > 0 AND last_completed_at_ms < sqlc.arg(before_ms)
  AND last_attention_at_ms < sqlc.arg(before_ms) AND updated_at_ms < sqlc.arg(before_ms)
  AND NOT EXISTS (
    SELECT 1 FROM bot_commitment_workers l JOIN bot_commitments c ON c.id = l.commitment_id
    WHERE l.thread_id = threads.id AND (c.status IN ('pending', 'in_progress', 'waiting_for_user', 'stopped') OR c.closed_at_ms >= sqlc.arg(before_ms))
  );
