-- +goose Up
DROP VIEW bot_commitment_watch;
DROP VIEW bot_commitment_progress;
DROP VIEW bot_commitment_thread_states;
CREATE VIEW bot_commitment_workers AS
WITH RECURSIVE work AS (
  SELECT commitment_id, id AS linked, thread_id FROM bot_commitment_threads
  UNION
  SELECT w.commitment_id, w.linked, t.id FROM threads t
  JOIN work w ON t.parent_id = w.thread_id
  JOIN bot_commitments c ON c.id = w.commitment_id
  WHERE t.source_type = 'bot_worker' AND t.source_id = c.bot_id
)
SELECT * FROM work;
CREATE VIEW bot_commitment_thread_states AS
SELECT w.commitment_id, w.linked, t.id AS thread_id, t.slug, t.title,
  CASE
    WHEN t.archived <> 0 THEN 'gone'
    WHEN c.status = 'stopped' THEN CASE WHEN t.last_completed_at_ms > 0 THEN 'finished' ELSE 'idle' END
    WHEN t.status IN ('running', 'interrupted') OR json_array_length(COALESCE(NULLIF(t.queued_messages, ''), '[]')) > 0 THEN 'working'
    WHEN t.background_work <> 0 THEN 'background'
    WHEN t.status = 'error' THEN 'failed'
    WHEN t.last_completed_at_ms > 0 THEN 'finished'
    ELSE 'idle'
  END AS state
FROM bot_commitment_workers w JOIN threads t ON t.id = w.thread_id
JOIN bot_commitments c ON c.id = w.commitment_id;

CREATE VIEW bot_commitment_progress AS
SELECT c.id AS commitment_id,
  count(t.id) || ':' || coalesce(sum(t.turns_ended), 0) AS progress,
  coalesce(max(s.state IN ('working', 'background')), 0) AS moving,
  EXISTS (
    SELECT 1 FROM bot_commitment_dependencies d JOIN bot_commitments p ON p.id = d.prerequisite_id
    WHERE d.commitment_id = c.id AND p.status IN ('pending', 'in_progress', 'waiting_for_user', 'stopped')
  ) AS blocked
FROM bot_commitments c
LEFT JOIN bot_commitment_thread_states s ON s.commitment_id = c.id
LEFT JOIN threads t ON t.id = s.thread_id
WHERE c.status IN ('pending', 'in_progress', 'waiting_for_user', 'stopped')
GROUP BY c.id;

CREATE VIEW bot_commitment_watch AS
SELECT c.id, c.bot_id, c.wake_progress, c.wake_attempts, c.wake_at_ms, c.check_at_ms, p.progress
FROM bot_commitments c
JOIN bot_commitment_progress p ON p.commitment_id = c.id
JOIN threads b ON b.id = c.bot_id
WHERE c.status IN ('pending', 'in_progress') AND NOT p.moving AND NOT p.blocked
  AND b.archived = 0 AND b.background_work = 0 AND b.status IN ('idle', 'error') AND b.turn = ''
  AND json_array_length(COALESCE(NULLIF(b.queued_messages, ''), '[]')) = 0;

-- +goose StatementBegin
CREATE TRIGGER bot_task_admit_insert AFTER INSERT ON threads
WHEN NEW.status = 'running' OR NEW.background_work <> 0
BEGIN
  SELECT RAISE(ABORT, 'a stopped task cannot run; explicitly resume it first')
  FROM bot_commitment_workers w JOIN bot_commitments c ON c.id = w.commitment_id
  WHERE w.thread_id = NEW.id AND c.status = 'stopped';
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER bot_task_admit_update AFTER UPDATE OF status, background_work, parent_id, source_type, source_id ON threads
WHEN NEW.status = 'running' OR NEW.background_work <> 0
BEGIN
  SELECT RAISE(ABORT, 'a stopped task cannot run; explicitly resume it first')
  FROM bot_commitment_workers w JOIN bot_commitments c ON c.id = w.commitment_id
  WHERE w.thread_id = NEW.id AND c.status = 'stopped';
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER bot_task_admit_insert;
DROP TRIGGER bot_task_admit_update;
DROP VIEW bot_commitment_watch;
DROP VIEW bot_commitment_progress;
DROP VIEW bot_commitment_thread_states;
DROP VIEW bot_commitment_workers;
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
SELECT c.id, c.bot_id, c.wake_progress, c.wake_attempts, c.wake_at_ms, c.check_at_ms, p.progress
FROM bot_commitments c
JOIN bot_commitment_progress p ON p.commitment_id = c.id
JOIN threads b ON b.id = c.bot_id
WHERE c.status IN ('pending', 'in_progress') AND NOT p.moving AND NOT p.blocked
  AND b.archived = 0 AND b.background_work = 0 AND b.status IN ('idle', 'error') AND b.turn = ''
  AND json_array_length(COALESCE(NULLIF(b.queued_messages, ''), '[]')) = 0;
