-- +goose Up
DROP TRIGGER bot_task_user_answer;
UPDATE bot_commitments SET status = 'pending', question_seq = 0, wake_at_ms = 0, wake_attempts = 0, wake_progress = ''
WHERE status = 'waiting_for_user' AND EXISTS (
  SELECT 1 FROM messages m JOIN session_events q ON q.thread_id = m.thread_id AND q.seq = bot_commitments.question_seq
  WHERE m.thread_id = bot_id AND m.from_user <> 0 AND m.created_at_ms > q.created_at_ms
);
ALTER TABLE bot_commitments DROP COLUMN question_user_seq;

DROP VIEW bot_commitment_thread_states;

CREATE VIEW bot_commitment_thread_states AS
WITH RECURSIVE work AS (
  SELECT commitment_id, id AS linked, thread_id FROM bot_commitment_threads
  UNION
  SELECT w.commitment_id, w.linked, t.id FROM threads t
  JOIN work w ON t.parent_id = w.thread_id
  JOIN bot_commitments c ON c.id = w.commitment_id
  WHERE t.source_type = 'bot_worker' AND t.source_id = c.bot_id
)
SELECT w.commitment_id, w.linked, t.id AS thread_id, t.slug, t.title,
  CASE
    WHEN t.archived <> 0 THEN 'gone'
    WHEN t.status IN ('running', 'interrupted') OR json_array_length(COALESCE(NULLIF(t.queued_messages, ''), '[]')) > 0 THEN 'working'
    WHEN t.background_work <> 0 THEN 'background'
    WHEN t.status = 'error' THEN 'failed'
    WHEN t.last_completed_at_ms > 0 THEN 'finished'
    ELSE 'idle'
  END AS state
FROM work w JOIN threads t ON t.id = w.thread_id;

-- +goose Down
DROP VIEW bot_commitment_thread_states;

CREATE VIEW bot_commitment_thread_states AS
SELECT l.commitment_id, l.id AS linked, t.id AS thread_id, t.slug, t.title,
  CASE
    WHEN t.archived <> 0 THEN 'gone'
    WHEN t.status IN ('running', 'interrupted') OR json_array_length(COALESCE(NULLIF(t.queued_messages, ''), '[]')) > 0 THEN 'working'
    WHEN t.background_work <> 0 THEN 'background'
    WHEN t.status = 'error' THEN 'failed'
    WHEN t.last_completed_at_ms > 0 THEN 'finished'
    ELSE 'idle'
  END AS state
FROM bot_commitment_threads l JOIN threads t ON t.id = l.thread_id;

ALTER TABLE bot_commitments ADD COLUMN question_user_seq INTEGER NOT NULL DEFAULT 0;
UPDATE bot_commitments SET question_user_seq = coalesce((SELECT max(seq) FROM messages WHERE thread_id = bot_id), 0)
WHERE status = 'waiting_for_user';

-- +goose StatementBegin
CREATE TRIGGER bot_task_user_answer AFTER INSERT ON messages
WHEN NEW.from_user <> 0
BEGIN
  UPDATE bot_commitments SET status = 'pending', question_seq = 0, question_user_seq = 0,
    wake_at_ms = 0, wake_attempts = 0, wake_progress = ''
  WHERE bot_id = NEW.thread_id AND status = 'waiting_for_user' AND NEW.seq > question_user_seq;
END;
-- +goose StatementEnd
