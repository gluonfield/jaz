-- +goose Up
-- Existing routines keep running in their bot's thread; new ones default to a
-- child thread.
ALTER TABLE loops ADD COLUMN thread TEXT NOT NULL DEFAULT 'main';

-- +goose Down
ALTER TABLE loops DROP COLUMN thread;
