-- +goose Up
ALTER TABLE threads ADD COLUMN cached_write_tokens INTEGER NOT NULL DEFAULT 0;

-- Move to disjoint token components (input = fresh, uncached input). Jaz ACP
-- rows were stored OpenAI-style with cache reads counted inside input, so
-- the subtraction is exact there.
UPDATE threads
SET input_tokens = input_tokens - cached_input_tokens
WHERE runtime = 'acp' AND input_tokens >= cached_input_tokens;

-- +goose Down
-- NOTE: the input_tokens fixup above is lossy and cannot be inverted; a
-- down+up cycle re-runs it and subtracts cached reads a second time.
ALTER TABLE threads DROP COLUMN cached_write_tokens;
