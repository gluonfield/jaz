-- +goose Up
-- Which version of the thread's own prompt its agent was shown, as of its
-- last successful turn; unknown after a compact, which rebuilds the agent's
-- context from the prompt its process was launched with.
ALTER TABLE threads ADD COLUMN prompt_digest TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE threads DROP COLUMN prompt_digest;
