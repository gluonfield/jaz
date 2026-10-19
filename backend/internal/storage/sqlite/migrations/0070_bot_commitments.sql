-- +goose Up
-- What a bot owes beyond the turn it was asked in, and the threads it started
-- for each; a thread serves at most one.
CREATE TABLE bot_commitments (
  id INTEGER PRIMARY KEY,
  bot_id TEXT NOT NULL,
  requester TEXT NOT NULL DEFAULT '',
  request TEXT NOT NULL,
  note TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'open',
  opened_at_ms INTEGER NOT NULL,
  closed_at_ms INTEGER
);
CREATE INDEX idx_bot_commitments_bot ON bot_commitments(bot_id, status);

CREATE TABLE bot_commitment_threads (
  id INTEGER PRIMARY KEY,
  thread_id TEXT NOT NULL UNIQUE,
  commitment_id INTEGER NOT NULL REFERENCES bot_commitments(id)
);
CREATE INDEX idx_bot_commitment_threads_commitment ON bot_commitment_threads(commitment_id);

-- How far each thread serving a commitment has got. A thread works while it
-- runs, waits to resume or has turns queued.
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

-- +goose Down
DROP VIEW bot_commitment_thread_states;
DROP TABLE bot_commitment_threads;
DROP TABLE bot_commitments;
