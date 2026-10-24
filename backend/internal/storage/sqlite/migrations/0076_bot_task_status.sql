-- +goose Up
DROP VIEW bot_commitment_watch;
DROP VIEW bot_commitment_progress;

ALTER TABLE bot_commitments ADD COLUMN question_seq INTEGER NOT NULL DEFAULT 0;
ALTER TABLE bot_commitments ADD COLUMN question_user_seq INTEGER NOT NULL DEFAULT 0;
ALTER TABLE bot_commitments ADD COLUMN wake_progress TEXT NOT NULL DEFAULT '';
ALTER TABLE bot_commitments ADD COLUMN wake_attempts INTEGER NOT NULL DEFAULT 0;
ALTER TABLE bot_commitments ADD COLUMN wake_at_ms INTEGER NOT NULL DEFAULT 0;

-- Legacy waits have no question or dependency evidence and must be reconciled.
UPDATE bot_commitments SET status = CASE status
  WHEN 'done' THEN 'completed'
  WHEN 'dropped' THEN 'cancelled'
  ELSE CASE WHEN EXISTS (SELECT 1 FROM bot_commitment_threads WHERE commitment_id = bot_commitments.id)
    THEN 'in_progress' ELSE 'pending' END
END;
ALTER TABLE bot_commitments DROP COLUMN waiting;
ALTER TABLE bot_commitments DROP COLUMN watch;

CREATE TABLE bot_commitment_dependencies (
  commitment_id INTEGER NOT NULL REFERENCES bot_commitments(id) ON DELETE CASCADE,
  prerequisite_id INTEGER NOT NULL REFERENCES bot_commitments(id) ON DELETE CASCADE,
  PRIMARY KEY (commitment_id, prerequisite_id),
  CHECK (commitment_id <> prerequisite_id)
);

CREATE VIEW bot_commitment_progress AS
SELECT c.id AS commitment_id,
  count(t.id) || ':' || coalesce(sum(t.turns_ended), 0) AS progress,
  coalesce(max(s.state IN ('working', 'background')), 0) AS moving,
  EXISTS (
    SELECT 1 FROM bot_commitment_dependencies d JOIN bot_commitments p ON p.id = d.prerequisite_id
    WHERE d.commitment_id = c.id AND p.status IN ('pending', 'in_progress', 'waiting_for_user')
  ) AS blocked
FROM bot_commitments c
LEFT JOIN bot_commitment_thread_states s ON s.commitment_id = c.id
LEFT JOIN threads t ON t.id = s.thread_id
WHERE c.status IN ('pending', 'in_progress', 'waiting_for_user')
GROUP BY c.id;

CREATE VIEW bot_commitment_watch AS
SELECT c.id, c.bot_id, c.wake_progress, c.wake_attempts, c.wake_at_ms, p.progress
FROM bot_commitments c
JOIN bot_commitment_progress p ON p.commitment_id = c.id
JOIN threads b ON b.id = c.bot_id
WHERE c.status IN ('pending', 'in_progress') AND NOT p.moving AND NOT p.blocked
  AND b.archived = 0 AND b.background_work = 0 AND b.status IN ('idle', 'error') AND b.turn = ''
  AND json_array_length(COALESCE(NULLIF(b.queued_messages, ''), '[]')) = 0;

-- +goose StatementBegin
CREATE TRIGGER bot_task_user_answer AFTER INSERT ON messages
WHEN NEW.from_user <> 0
BEGIN
  UPDATE bot_commitments SET status = 'pending', question_seq = 0, question_user_seq = 0,
    wake_at_ms = 0, wake_attempts = 0, wake_progress = ''
  WHERE bot_id = NEW.thread_id AND status = 'waiting_for_user' AND NEW.seq > question_user_seq;
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER bot_task_user_answer;
DROP VIEW bot_commitment_watch;
DROP VIEW bot_commitment_progress;
DROP TABLE bot_commitment_dependencies;
ALTER TABLE bot_commitments ADD COLUMN waiting TEXT NOT NULL DEFAULT '';
ALTER TABLE bot_commitments ADD COLUMN watch TEXT NOT NULL DEFAULT '';
UPDATE bot_commitments SET status = CASE status WHEN 'completed' THEN 'done' WHEN 'cancelled' THEN 'dropped' ELSE 'open' END;
ALTER TABLE bot_commitments DROP COLUMN question_seq;
ALTER TABLE bot_commitments DROP COLUMN question_user_seq;
ALTER TABLE bot_commitments DROP COLUMN wake_progress;
ALTER TABLE bot_commitments DROP COLUMN wake_attempts;
ALTER TABLE bot_commitments DROP COLUMN wake_at_ms;
CREATE VIEW bot_commitment_progress AS
SELECT c.id AS commitment_id, count(t.id) || ':' || coalesce(sum(t.turns_ended), 0) AS progress,
  coalesce(max(s.state IN ('working', 'background')), 0) AS moving
FROM bot_commitments c
LEFT JOIN bot_commitment_thread_states s ON s.commitment_id = c.id
LEFT JOIN threads t ON t.id = s.thread_id
WHERE c.status = 'open' GROUP BY c.id;
CREATE VIEW bot_commitment_watch AS
SELECT c.id, c.bot_id, c.waiting, c.watch, p.progress, p.moving
FROM bot_commitments c JOIN bot_commitment_progress p ON p.commitment_id = c.id JOIN threads b ON b.id = c.bot_id
WHERE c.status = 'open' AND b.archived = 0 AND b.status IN ('idle', 'error') AND b.turn = ''
  AND json_array_length(COALESCE(NULLIF(b.queued_messages, ''), '[]')) = 0;
