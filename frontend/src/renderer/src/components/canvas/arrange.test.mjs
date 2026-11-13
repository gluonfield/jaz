import { expect, test } from 'bun:test'
import { alignItems, layerItems, styleOf } from '@/components/canvas/arrange'
import { toFlowNodes } from '@/components/canvas/board'

test('aligning moves each item to an edge or centre line of the box around them', () => {
  const items = [
    { id: 'a', x: 0, y: 0, width: 100, height: 40 },
    { id: 'b', x: 300, y: 100, width: 50, height: 100 },
  ]
  expect(alignItems(items, 'left')).toEqual([{ id: 'a', x: 0 }, { id: 'b', x: 0 }])
  expect(alignItems(items, 'center')).toEqual([{ id: 'a', x: 125 }, { id: 'b', x: 150 }])
  expect(alignItems(items, 'right')).toEqual([{ id: 'a', x: 250 }, { id: 'b', x: 300 }])
  expect(alignItems(items, 'top')).toEqual([{ id: 'a', y: 0 }, { id: 'b', y: 0 }])
  expect(alignItems(items, 'middle')).toEqual([{ id: 'a', y: 80 }, { id: 'b', y: 50 }])
  expect(alignItems(items, 'bottom')).toEqual([{ id: 'a', y: 160 }, { id: 'b', y: 100 }])
})

test('restacking lifts the picked items over the rest in their own order, once', () => {
  const items = [{ id: 'a' }, { id: 'b', z: 4 }, { id: 'c', z: -2 }, { id: 'd' }]
  expect(layerItems(items, new Set(['a', 'c']), 'front')).toEqual([{ id: 'a', z: 5 }, { id: 'c', z: 6 }])
  expect(layerItems(items, new Set(['b', 'd']), 'back')).toEqual([{ id: 'b', z: -4 }, { id: 'd', z: -3 }])
  expect(layerItems(items, new Set(['b']), 'front')).toEqual([])
  expect(layerItems(items, new Set(['c']), 'back')).toEqual([])
  expect(layerItems(items, new Set(['a', 'b', 'c', 'd']), 'front')).toEqual([])
})

test('a locked item cannot be dragged or deleted, and z sets the stacking order', () => {
  const nodes = toFlowNodes({ version: 'v', dir: '/c', edges: [], files: {}, components: {}, nodes: [
    { id: 'free', type: 'text', x: 0, y: 0, width: 100 },
    { id: 'held', type: 'text', x: 0, y: 0, width: 100, locked: true, z: 3 },
  ] })
  expect(nodes.map((node) => [node.id, node.draggable, node.deletable, node.zIndex])).toEqual([
    ['free', undefined, undefined, 0],
    ['held', false, false, 3],
  ])
})

test('a copied style carries the look and clears what the source leaves unset', () => {
  expect(styleOf({ id: 'a', type: 'text', text: 'Hi', x: 4, size: 'large', color: 'blue', shape: 'pill', hollow: true })).toEqual({ size: 'large', textAlign: null, ink: null, color: 'blue', shape: 'pill', hollow: true, weight: null, highlight: null })
})
