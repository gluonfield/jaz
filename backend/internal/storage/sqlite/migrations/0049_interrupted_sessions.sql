-- +goose Up
ALTER TABLE threads ADD COLUMN turn TEXT NOT NULL DEFAULT '';

UPDATE threads
SET status = 'interrupted', error = NULL
WHERE status = 'error'
  AND error = 'Server restarted while this thread was still running.';

-- +goose Down
UPDATE threads
SET status = 'error', error = 'Server restarted while this thread was still running.'
WHERE status = 'interrupted';
ALTER TABLE threads DROP COLUMN turn;
