import { expect, test } from 'bun:test'
import { layoutIssues, resolvePlaces } from '@/components/canvas/layout'

test('items placed beside others take their spot from measured sizes, anchors first', () => {
  const { moves, stuck } = resolvePlaces([
    { id: 'c', x: 0, y: 0, width: 100, height: 40, place: { right: 'b', align: 'center' } },
    { id: 'b', x: 0, y: 0, width: 200, height: 60, place: { below: 'a', gap: 24, align: 'center' } },
    { id: 'a', x: 100, y: 50, width: 300, height: 131 },
    { id: 'd', x: 0, y: 0, width: 80, height: 30, place: { above: 'a', align: 'end' } },
    { id: 'e', x: 0, y: 0, width: 80, height: 30, place: { left: 'a' } },
  ])
  expect(stuck).toEqual([])
  expect(moves).toEqual([
    { id: 'b', x: 150, y: 205, place: null },
    { id: 'd', x: 320, y: -20, place: null },
    { id: 'e', x: -20, y: 50, place: null },
    { id: 'c', x: 390, y: 215, place: null },
  ])
})

test('a place against a missing item, a loop or a malformed place moves nothing', () => {
  const { moves, stuck } = resolvePlaces([
    { id: 'a', x: 0, y: 0, width: 10, height: 10, place: { below: 'gone' } },
    { id: 'b', x: 0, y: 0, width: 10, height: 10, place: { below: 'c' } },
    { id: 'c', x: 0, y: 0, width: 10, height: 10, place: { right: 'b' } },
    { id: 'd', x: 0, y: 0, width: 10, height: 10, place: 'below a' },
    { id: 'e', x: 0, y: 0, width: 10, height: 10, place: { under: 'a' } },
  ])
  expect(moves).toEqual([])
  expect(stuck).toEqual([
    '`a` is placed against `gone`, which is not on this page',
    '`b` is placed against `c`, which waits on a place itself',
    '`c` is placed against `b`, which waits on a place itself',
  ])
})

test('issues name overlaps, items across a section edge and shrunk content, and nothing else', () => {
  expect(layoutIssues([
    { id: 'area', type: 'group', x: 0, y: 0, width: 400, height: 300 },
    { id: 'inside', type: 'text', x: 36, y: 36, width: 200, height: 80 },
    { id: 'edge', type: 'text', x: 300, y: 200, width: 200, height: 80, scale: 0.985 },
    { id: 'under', type: 'table', x: 100, y: 100, width: 150, height: 60, scale: 0.62 },
    { id: 'apart', type: 'text', x: 600, y: 0, width: 100, height: 40, scale: 1 },
    { id: 'touching', type: 'text', x: 700, y: 0, width: 100, height: 40 },
    { id: 'ink', type: 'stroke', x: 0, y: 0, width: 900, height: 400 },
    { id: 'outer', type: 'group', x: -50, y: -50, width: 1000, height: 500 },
  ])).toEqual([
    '`inside` overlaps `under` by 136 x 16 px',
    '`edge` crosses the edge of section `area`',
    '`under` is shrunk to 62% to fit its box',
  ])
  const crowd = Array.from({ length: 12 }, (_, index) => ({ id: `n${index}`, type: 'text', x: 0, y: 0, width: 10, height: 10 }))
  expect(layoutIssues(crowd)).toHaveLength(31)
  expect(layoutIssues(crowd).at(-1)).toBe('and 36 more')
})
