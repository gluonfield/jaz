import { describe, expect, test } from 'bun:test'
import { islandShape, mergeRows } from './islandPolicy'

const alert = (sessionId, state, at) => ({ sessionId, state, at, key: `${state}:${at}`, kind: 'chat' })

describe('island policy', () => {
  test('orders needs you, stopped, done, then replies, newest first, one row per thread', () => {
    let rows = []
    rows = mergeRows(rows, alert('a', 'reply', 1))
    rows = mergeRows(rows, alert('b', 'done', 2))
    rows = mergeRows(rows, alert('c', 'reply', 3))
    rows = mergeRows(rows, alert('d', 'needs_you', 4))
    rows = mergeRows(rows, alert('e', 'stopped', 5))
    expect(rows.map((row) => row.sessionId)).toEqual(['d', 'e', 'b', 'c', 'a'])
    rows = mergeRows(rows, alert('a', 'done', 6))
    expect(rows.map((row) => `${row.sessionId}:${row.state}`)).toEqual(['d:needs_you', 'e:stopped', 'a:done', 'b:done', 'c:reply'])
  })

  test('hugs only known notches and never draws on a possible Dynamic Island Mac', () => {
    expect(islandShape('Mac16,5', true, true)).toBe('notch')
    expect(islandShape('Mac16,5', true, false)).toBeNull()
    expect(islandShape('Mac18,1', true, true)).toBeNull()
    expect(islandShape('Mac18,1', true, false)).toBeNull()
    expect(islandShape('MacBookAir10,1', true, false)).toBe('capsule')
    expect(islandShape('Mac18,1', false, false)).toBe('capsule')
  })
})
