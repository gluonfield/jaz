import { expect, test } from 'bun:test'
import { fullRateInputTokens, inputTokens, peakDay, sumCategoryUsage, sumModelUsage, sumUsage, totalUsageTokens } from './usageDaily'

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

test('activity totals count new input and output, excluding cache reads', () => {
  for (const usage of [firstTurn, laterTurn, cumulative]) {
    expect(inputTokens(usage) + usage.output_tokens).toBe(totalUsageTokens(usage))
  }
})

test('daily, model, and activity breakdowns rank work consistently despite different cache hit rates', () => {
  const chat = { input_tokens: 579_178, cached_input_tokens: 519_424, output_tokens: 3_711 }
  const search = { input_tokens: 1_000_000, cached_input_tokens: 999_000, output_tokens: 200 }
  const days = [
    { date: '2026-09-09', usage: chat, categories: [{ category: 'chat', usage: chat }], models: [{ model: 'chat', usage: chat }] },
    { date: '2026-09-10', usage: search, categories: [{ category: 'memory_search', usage: search }], models: [{ model: 'search', usage: search }] },
  ]
  expect(sumUsage(days).input_output_tokens).toBe(64_665)
  expect(sumCategoryUsage(days).map((row) => [row.category, row.usage.input_output_tokens])).toEqual([
    ['chat', 63_465],
    ['memory_search', 1_200],
  ])
  expect(sumModelUsage(days).map((row) => [row.model, row.usage.input_output_tokens])).toEqual([
    ['chat', 63_465],
    ['search', 1_200],
  ])
  expect(peakDay(days).date).toBe('2026-09-09')
})

test('cost splits input into the full-rate slice and the cache write it paid for', () => {
  expect(fullRateInputTokens(firstTurn)).toBe(6_090)
  expect(fullRateInputTokens(laterTurn)).toBe(10)
  for (const usage of [firstTurn, laterTurn, cumulative]) {
    expect(fullRateInputTokens(usage) + usage.cached_write_tokens).toBe(inputTokens(usage))
  }
})
