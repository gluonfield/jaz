-- +goose Up
-- Return usage rows to provider-facing input semantics: input includes cache
-- read/write tokens, while cache columns remain detail fields.
UPDATE threads
SET input_tokens = input_tokens + cached_input_tokens + cached_write_tokens
WHERE (cached_input_tokens > 0 OR cached_write_tokens > 0)
  AND total_tokens = input_tokens + cached_input_tokens + cached_write_tokens + output_tokens;

UPDATE usage_events
SET input_tokens = input_tokens + cached_input_tokens + cached_write_tokens
WHERE (cached_input_tokens > 0 OR cached_write_tokens > 0)
  AND total_tokens = input_tokens + cached_input_tokens + cached_write_tokens + output_tokens;

-- +goose Down
UPDATE threads
SET input_tokens = input_tokens - cached_input_tokens - cached_write_tokens
WHERE (cached_input_tokens > 0 OR cached_write_tokens > 0)
  AND input_tokens >= cached_input_tokens + cached_write_tokens
  AND total_tokens = input_tokens + output_tokens;

UPDATE usage_events
SET input_tokens = input_tokens - cached_input_tokens - cached_write_tokens
WHERE (cached_input_tokens > 0 OR cached_write_tokens > 0)
  AND input_tokens >= cached_input_tokens + cached_write_tokens
  AND total_tokens = input_tokens + output_tokens;
