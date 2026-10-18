-- +goose Up
-- Whether a run sent or presented its outcome, or reported it to its bot.
ALTER TABLE loop_runs ADD COLUMN delivered INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE loop_runs DROP COLUMN delivered;
