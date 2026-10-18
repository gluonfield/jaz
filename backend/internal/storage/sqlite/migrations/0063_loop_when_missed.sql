-- +goose Up
ALTER TABLE loops ADD COLUMN when_missed TEXT NOT NULL DEFAULT 'skip';

CREATE TABLE loop_events (
  loop_id TEXT NOT NULL,
  event_id TEXT NOT NULL,
  run_id TEXT NOT NULL,
  at_ms INTEGER NOT NULL,
  PRIMARY KEY (loop_id, event_id)
);

-- +goose Down
DROP TABLE loop_events;
ALTER TABLE loops DROP COLUMN when_missed;
