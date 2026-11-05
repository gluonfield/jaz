-- +goose Up
CREATE TEMP TABLE task_threads AS SELECT id, thread_id, commitment_id FROM bot_commitment_threads;
DROP TABLE bot_commitment_threads;
CREATE TABLE bot_commitment_threads (
  id INTEGER PRIMARY KEY,
  thread_id TEXT NOT NULL,
  commitment_id INTEGER NOT NULL REFERENCES bot_commitments(id),
  UNIQUE (commitment_id, thread_id)
);
INSERT INTO bot_commitment_threads SELECT id, thread_id, commitment_id FROM task_threads;
DROP TABLE task_threads;

-- +goose Down
CREATE TEMP TABLE task_threads AS SELECT min(id) AS id, thread_id, commitment_id FROM bot_commitment_threads GROUP BY thread_id;
DROP TABLE bot_commitment_threads;
CREATE TABLE bot_commitment_threads (
  id INTEGER PRIMARY KEY,
  thread_id TEXT NOT NULL UNIQUE,
  commitment_id INTEGER NOT NULL REFERENCES bot_commitments(id)
);
CREATE INDEX idx_bot_commitment_threads_commitment ON bot_commitment_threads(commitment_id);
INSERT INTO bot_commitment_threads SELECT id, thread_id, commitment_id FROM task_threads;
DROP TABLE task_threads;
