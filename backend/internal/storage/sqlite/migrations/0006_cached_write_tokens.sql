-- +goose Up
ALTER TABLE threads ADD COLUMN cached_write_tokens INTEGER NOT NULL DEFAULT 0;

-- Move to disjoint token components (input = fresh, uncached input). Native
-- rows were stored OpenAI-style with cache reads counted inside input, so
-- the subtraction is exact there. ACP rows are left alone: pre-fix rows were
-- already disjoint, and the few folded ones can't be unfolded.
UPDATE threads
SET input_tokens = input_tokens - cached_input_tokens
WHERE runtime = 'native' AND input_tokens >= cached_input_tokens;

-- +goose Down
-- NOTE: the input_tokens fixup above is lossy and cannot be inverted; a
-- down+up cycle re-runs it and subtracts cached reads a second time.
ALTER TABLE threads DROP COLUMN cached_write_tokens;
