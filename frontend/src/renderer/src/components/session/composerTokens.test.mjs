import { describe, expect, test } from 'bun:test'
import { backwardDeleteRange, tokenEndingAt } from './composerTokens'

function applyBackspace(value, start, end = start, modifiers = {}) {
  const range = backwardDeleteRange(value, start, end, modifiers)
  if (!range) return { value, caret: start }
  return {
    value: value.slice(0, range.from) + value.slice(range.to),
    caret: range.from,
  }
}

describe('backwardDeleteRange', () => {
  test('deletes one character or grapheme cluster before the caret', () => {
    expect(applyBackspace('hello', 5)).toEqual({ value: 'hell', caret: 4 })
    expect(applyBackspace('hello', 2)).toEqual({ value: 'hllo', caret: 1 })
    expect(applyBackspace('hello', 0)).toEqual({ value: 'hello', caret: 0 })
    expect(applyBackspace('hi 👋🏽', 7)).toEqual({ value: 'hi ', caret: 3 })
  })

  test('deletes the active selection regardless of direction', () => {
    expect(applyBackspace('hello world', 2, 7)).toEqual({ value: 'heorld', caret: 2 })
    expect(applyBackspace('hello world', 7, 2)).toEqual({ value: 'heorld', caret: 2 })
  })

  test('deletes the previous word with Option/Alt+Backspace', () => {
    expect(applyBackspace('hello world', 11, 11, { alt: true })).toEqual({ value: 'hello ', caret: 6 })
    expect(applyBackspace('hello   ', 8, 8, { alt: true })).toEqual({ value: '', caret: 0 })
    expect(applyBackspace('foo.bar', 7, 7, { alt: true })).toEqual({ value: 'foo.', caret: 4 })
    expect(applyBackspace('line1\nword', 10, 10, { alt: true })).toEqual({ value: 'line1\n', caret: 6 })
    expect(applyBackspace('line1\n', 6, 6, { alt: true })).toEqual({ value: 'line1', caret: 5 })
  })

  test('deletes to the start of the line with Cmd+Backspace', () => {
    expect(applyBackspace('first\nsecond', 12, 12, { meta: true })).toEqual({ value: 'first\n', caret: 6 })
    expect(applyBackspace('first\n', 6, 6, { meta: true })).toEqual({ value: 'first', caret: 5 })
    expect(applyBackspace('single line', 6, 6, { meta: true })).toEqual({ value: ' line', caret: 0 })
  })
})

describe('tokenEndingAt', () => {
  test('matches only when the caret sits at the end of a known token', () => {
    const tokens = new Map([
      ['@src/app.ts', { trigger: '@', display: '@src/app.ts', expansion: '[@src/app.ts](/repo/src/app.ts)' }],
    ])
    const text = 'check @src/app.ts please'
    expect(tokenEndingAt(text, 17, tokens)).toEqual({
      token: tokens.get('@src/app.ts'),
      start: 6,
    })
    expect(tokenEndingAt(text, 18, tokens)).toBeNull()
    expect(tokenEndingAt(text, 5, tokens)).toBeNull()
  })
})
