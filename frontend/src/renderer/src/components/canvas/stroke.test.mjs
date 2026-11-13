import { expect, test } from 'bun:test'
import { penOf, strokeItem, strokePath, strokePoints } from '@/components/canvas/stroke'

test('a drawn line becomes an item boxed around its points, without the crowded ones or default pen settings', () => {
  const item = strokeItem([[100.4, 50.2], [100.9, 50.4], [140, 90.26], [180.5, 60]], { weight: 4, highlight: false })
  expect(item).toEqual({ type: 'stroke', x: 100, y: 50, width: 81, height: 41, points: [[0.4, 0.2], [40, 40.3], [80.5, 10]] })
  expect(strokeItem([[5, 5]], { color: 'red', weight: 8, highlight: true })).toEqual({ type: 'stroke', x: 5, y: 5, width: 1, height: 1, points: [[0, 0]], color: 'red', weight: 8, highlight: true })
  expect(strokeItem([[0, 0], [0.5, 0], [1, 0]], { weight: 4, highlight: false }).points).toEqual([[0, 0], [1, 0]])
})

test('a line reads back with its pen, ignoring points and settings that are not numbers', () => {
  const item = { id: 'a', type: 'stroke', points: [[0, 0], 'x', [3], [4, 5]], color: 'blue', weight: -1, highlight: 'yes' }
  expect(strokePoints(item)).toEqual([[0, 0], [4, 5]])
  expect(penOf(item)).toEqual({ color: 'blue', weight: 4, highlight: false })
  expect(strokePoints({ id: 'b', type: 'stroke' })).toEqual([])
})

test('the path starts and ends on the drawn points and bends through the ones between', () => {
  expect(strokePath([])).toBe('')
  expect(strokePath([[2, 3]])).toBe('M2 3h0.01')
  expect(strokePath([[0, 0], [10, 0]])).toBe('M0 0L5 0L10 0')
  expect(strokePath([[0, 0], [10, 0], [10, 10]])).toBe('M0 0L5 0Q10 0 10 5L10 10')
})
