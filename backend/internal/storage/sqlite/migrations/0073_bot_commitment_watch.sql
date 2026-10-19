-- +goose Up
-- What an open commitment waits on, as its bot said: '' for nothing,
-- 'requester' or 'outside'. Watch is what Jaz last did about it stalling, at
-- the progress it then had: '' for nothing, 'woke:<progress>' after waking
-- its bot once, 'stalled:<progress>' after telling the user it stalled.
ALTER TABLE bot_commitments ADD COLUMN waiting TEXT NOT NULL DEFAULT '';
ALTER TABLE bot_commitments ADD COLUMN watch TEXT NOT NULL DEFAULT '';

-- How many of its recorded turns a thread has ended, however each ended.
ALTER TABLE threads ADD COLUMN turns_ended INTEGER NOT NULL DEFAULT 0;

-- Whether a thread's agent holds a live background task, as the ACP manager
-- last saw it. No agent process outlives the server, which clears it as it
-- starts.
ALTER TABLE threads ADD COLUMN background_work INTEGER NOT NULL DEFAULT 0;

-- A thread whose turn ended while its agent still runs a background task
-- works in the background: it moves its commitment on, which can still close.
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

-- How far an open commitment's threads have got, as how many it has and how
-- many turns they ended, and whether one of them works on it.
CREATE VIEW bot_commitment_progress AS
SELECT c.id AS commitment_id,
  count(t.id) || ':' || coalesce(sum(t.turns_ended), 0) AS progress,
  coalesce(max(s.state IN ('working', 'background')), 0) AS moving
FROM bot_commitments c
LEFT JOIN bot_commitment_thread_states s ON s.commitment_id = c.id
LEFT JOIN threads t ON t.id = s.thread_id
WHERE c.status = 'open'
GROUP BY c.id;

-- The open commitments of bots whose own thread has ended its turn with
-- nothing queued: nothing of theirs moves unless a thread does.
CREATE VIEW bot_commitment_watch AS
SELECT c.id, c.bot_id, c.waiting, c.watch, p.progress, p.moving
FROM bot_commitments c
JOIN bot_commitment_progress p ON p.commitment_id = c.id
JOIN threads b ON b.id = c.bot_id
WHERE c.status = 'open' AND b.archived = 0 AND b.status IN ('idle', 'error') AND b.turn = ''
  AND json_array_length(COALESCE(NULLIF(b.queued_messages, ''), '[]')) = 0;

-- +goose Down
DROP VIEW bot_commitment_watch;
DROP VIEW bot_commitment_progress;
DROP VIEW bot_commitment_thread_states;
CREATE VIEW bot_commitment_thread_states AS
SELECT l.commitment_id, l.id AS linked, t.id AS thread_id, t.slug, t.title,
  CASE
    WHEN t.archived <> 0 THEN 'gone'
    WHEN t.status IN ('running', 'interrupted') OR json_array_length(COALESCE(NULLIF(t.queued_messages, ''), '[]')) > 0 THEN 'working'
    WHEN t.status = 'error' THEN 'failed'
    WHEN t.last_completed_at_ms > 0 THEN 'finished'
    ELSE 'idle'
  END AS state
FROM bot_commitment_threads l JOIN threads t ON t.id = l.thread_id;
ALTER TABLE threads DROP COLUMN background_work;
ALTER TABLE threads DROP COLUMN turns_ended;
ALTER TABLE bot_commitments DROP COLUMN watch;
ALTER TABLE bot_commitments DROP COLUMN waiting;
