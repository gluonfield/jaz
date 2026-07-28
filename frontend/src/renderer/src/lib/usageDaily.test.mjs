import { expect, test } from 'bun:test'
import { fullRateInputTokens, inputTokens, totalUsageTokens } from './usageDaily'

// The backend stores input inclusive of cache (normalizeInclusiveInput), so
// these three turns are what a real Claude thread accumulates.
const firstTurn = { input_tokens: 23_514, cached_input_tokens: 0, cached_write_tokens: 17_424, output_tokens: 15 }
const laterTurn = { input_tokens: 22_113, cached_input_tokens: 22_000, cached_write_tokens: 103, output_tokens: 119 }
const cumulative = {
  input_tokens: firstTurn.input_tokens + laterTurn.input_tokens,
  cached_input_tokens: firstTurn.cached_input_tokens + laterTurn.cached_input_tokens,
  cached_write_tokens: firstTurn.cached_write_tokens + laterTurn.cached_write_tokens,
  output_tokens: firstTurn.output_tokens + laterTurn.output_tokens,
}

test('input is every token sent that was not replayed from cache', () => {
  expect(inputTokens(firstTurn)).toBe(23_514)
  expect(inputTokens(laterTurn)).toBe(113)
  expect(inputTokens(cumulative)).toBe(23_627)
})

test('input only grows as turns accumulate', () => {
  expect(inputTokens(cumulative)).toBe(inputTokens(firstTurn) + inputTokens(laterTurn))
  expect(inputTokens(cumulative)).toBeGreaterThan(inputTokens(firstTurn))
})

test('input plus cache reads plus output is the total', () => {
  for (const usage of [firstTurn, laterTurn, cumulative]) {
    expect(inputTokens(usage) + usage.cached_input_tokens + usage.output_tokens).toBe(totalUsageTokens(usage))
  }
})

test('cost splits input into the full-rate slice and the cache write it paid for', () => {
  expect(fullRateInputTokens(firstTurn)).toBe(6_090)
  expect(fullRateInputTokens(laterTurn)).toBe(10)
  for (const usage of [firstTurn, laterTurn, cumulative]) {
    expect(fullRateInputTokens(usage) + usage.cached_write_tokens).toBe(inputTokens(usage))
  }
})
