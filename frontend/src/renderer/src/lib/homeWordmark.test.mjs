import { describe, expect, test } from 'bun:test'
import {
  DEFAULT_HOME_WORDMARK,
  effectiveHomeWordmark,
  HOME_WORDMARK_MAX_LENGTH,
  normalizeHomeWordmark,
} from './homeWordmark'

describe('home wordmark', () => {
  test('uses jaz when the appearance override is blank', () => {
    expect(effectiveHomeWordmark('   ')).toBe(DEFAULT_HOME_WORDMARK)
  })

  test('trims custom text and preserves internal spaces', () => {
    expect(effectiveHomeWordmark('  hello world  ')).toBe('hello world')
  })

  test('caps custom text by Unicode character rather than UTF-16 code unit', () => {
    const text = '🪐'.repeat(HOME_WORDMARK_MAX_LENGTH + 1)
    expect(Array.from(normalizeHomeWordmark(text))).toHaveLength(HOME_WORDMARK_MAX_LENGTH)
  })
})
