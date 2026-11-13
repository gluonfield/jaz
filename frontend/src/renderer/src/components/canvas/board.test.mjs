import { expect, test } from 'bun:test'
import { applyPatch, changedNodeIds, cloneItems, innermostSection, invertPatch, mergeNodes, nextTo, sectionContents, toFlowEdges, toFlowNodes, withContents } from '@/components/canvas/board'

const view = (nodes, edges = [], files = {}) => ({ version: 'v', dir: '/c', nodes, edges, files, components: {} })

test('optimistic patches follow the server: merge by id, null removes, removed nodes take their edges', () => {
  const before = view(
    [
      { id: 'a', type: 'text', text: 'agent', x: 0, y: 0, width: 100, height: 100, styleAttributes: { shape: 'pill' } },
      { id: 'b', type: 'text', text: 'b', x: 200, y: 0, width: 100, height: 100 },
      { id: 'c', type: 'text', text: 'c', x: 400, y: 0, width: 100, height: 100 },
    ],
    [
      { id: 'ab', fromNode: 'a', toNode: 'b', label: 'next' },
      { id: 'bc', fromNode: 'b', toNode: 'c' },
    ],
  )
  const after = applyPatch(before, {
    nodes: [{ id: 'a', x: 40 }, { id: 'd', type: 'group', label: 'Section', x: 0, y: 0, width: 300, height: 200 }],
    edges: [{ id: 'ab', label: null }],
    remove: ['c'],
  })
  expect(after.nodes.map((node) => node.id)).toEqual(['a', 'b', 'd'])
  expect(after.nodes[0]).toEqual({ id: 'a', type: 'text', text: 'agent', x: 40, y: 0, width: 100, height: 100, styleAttributes: { shape: 'pill' } })
  expect(after.edges).toEqual([{ id: 'ab', fromNode: 'a', toNode: 'b' }])
  expect(before.nodes[0].x).toBe(0)
})

test('server views replace the board, selected as asked, except an item being resized', () => {
  const server = toFlowNodes(view([
    { id: 'free', type: 'text', x: 500, y: 500, width: 100, height: 100 },
    { id: 'resized', type: 'text', x: 500, y: 500, width: 100, height: 100 },
    { id: 'new', type: 'text', x: 0, y: 0, width: 100, height: 100 },
  ]))
  const local = [
    { ...server[0], data: { item: { ...server[0].data.item } }, position: { x: 1, y: 1 } },
    { ...server[1], width: 300, resizing: true },
    { id: 'gone', type: 'text', position: { x: 0, y: 0 }, data: { item: { id: 'gone' } } },
  ]
  const merged = mergeNodes(local, server, new Set(['free', 'new']))
  expect(merged.map((node) => [node.id, node.position.x, node.width, Boolean(node.selected)])).toEqual([
    ['free', 500, 100, true],
    ['resized', 500, 300, false],
    ['new', 0, 100, true],
  ])
  expect(mergeNodes(merged, server, new Set(['free', 'new']))[0]).toBe(merged[0])
})

test('text, tables and documents leave their height to the board, limited by what the item reserves', () => {
  const nodes = toFlowNodes(view([
    { id: 'text', type: 'text', x: 0, y: 0, width: 100, height: 300 },
    { id: 'brief', type: 'file', file: 'brief.md', x: 0, y: 0, width: 100, height: 5000 },
    { id: 'open', type: 'file', file: 'costs.csv', x: 0, y: 0, width: 100 },
    { id: 'sticky', type: 'text', color: '3', x: 0, y: 0, width: 100, height: 100 },
    { id: 'part', type: 'file', file: 'part.step', x: 0, y: 0, width: 100, height: 100 },
    { id: 'block', type: 'text', shape: 'diamond', x: 0, y: 0, width: 100, height: 100 },
    { id: 'grid', type: 'table', rows: [['a']], x: 0, y: 0, width: 100 },
  ]))
  expect(nodes.map((node) => [node.id, node.height, node.style?.['--canvas-reserve']])).toEqual([
    ['text', undefined, '300px'],
    ['brief', undefined, '5000px'],
    ['open', undefined, 'none'],
    ['sticky', 100, undefined],
    ['part', 100, undefined],
    ['block', 100, undefined],
    ['grid', undefined, 'none'],
  ])
})

test('sections render behind items, connectors over all of them, with sides from geometry unless the agent named them', () => {
  const board = view(
    [
      { id: 'left', type: 'text', x: 0, y: 0, width: 100, height: 100 },
      { id: 'section', type: 'group', x: -50, y: -50, width: 600, height: 300 },
      { id: 'right', type: 'text', x: 400, y: 0, width: 100, height: 100 },
      { id: 'mystery', type: 'widget', x: 0, y: 0, width: 10, height: 10 },
    ],
    [
      { id: 'auto', fromNode: 'left', toNode: 'right' },
      { id: 'named', fromNode: 'left', toNode: 'right', fromSide: 'bottom', toSide: 'bottom', toEnd: 'none' },
      { id: 'dangling', fromNode: 'left', toNode: 'gone' },
    ],
  )
  expect(toFlowNodes(board).map((node) => node.id)).toEqual(['section', 'left', 'right'])
  const edges = toFlowEdges(board)
  expect(edges.map((edge) => edge.zIndex)).toEqual([1, 1])
  expect(toFlowEdges(view([{ ...board.nodes[0], z: 7 }, board.nodes[2]], board.edges))[0].zIndex).toBe(8)
  expect(edges.map((edge) => [edge.id, edge.sourceHandle, edge.targetHandle, Boolean(edge.markerEnd)])).toEqual([
    ['auto', 'right', 'left', true],
    ['named', 'bottom', 'bottom', false],
  ])
})

test('changes are what differs from what was shown: rewritten files count, field order does not', () => {
  const node = { id: 'pdf', type: 'file', file: 'quote.pdf', x: 0, y: 0, width: 300, height: 400 }
  const before = view([node, { id: 'note', type: 'text', text: 'a', x: 1 }], [], { pdf: { path: '/c/quote.pdf', version: '1' } })
  const after = view([node, { x: 1, text: 'a', type: 'text', id: 'note' }], [], { pdf: { path: '/c/quote.pdf', version: '2' } })
  expect(changedNodeIds(before, after)).toEqual(['pdf'])
  expect(changedNodeIds(after, after)).toEqual([])
})

test('a section holds what is centred inside it, never the section around it', () => {
  const nodes = toFlowNodes(view([
    { id: 'outer', type: 'group', x: 0, y: 0, width: 1000, height: 800 },
    { id: 'inner', type: 'group', x: 100, y: 100, width: 400, height: 300 },
    { id: 'in', type: 'text', x: 150, y: 150, width: 100, height: 100 },
    { id: 'straddling', type: 'text', x: 420, y: 150, width: 120, height: 100 },
    { id: 'out', type: 'text', x: 600, y: 150, width: 100, height: 100 },
  ]))
  const inner = nodes.find((node) => node.id === 'inner')
  const outer = nodes.find((node) => node.id === 'outer')
  expect(sectionContents(inner, nodes).map((node) => node.id)).toEqual(['in', 'straddling'])
  expect(sectionContents(outer, nodes).map((node) => node.id)).toEqual(['inner', 'in', 'straddling', 'out'])
  expect(withContents([inner, nodes.find((node) => node.id === 'in')], nodes).map((node) => node.id)).toEqual(['inner', 'in', 'straddling'])
})

test('the innermost section under a point wins', () => {
  const sections = [
    { id: 'outer', rect: { x: 0, y: 0, width: 1000, height: 800 } },
    { id: 'inner', rect: { x: 100, y: 100, width: 400, height: 300 } },
  ]
  expect(innermostSection(sections, 150, 150)?.id).toBe('inner')
  expect(innermostSection(sections, 700, 150)?.id).toBe('outer')
  expect(innermostSection(sections, 2000, 150)).toBeUndefined()
})

test('undo restores what a patch changed and keeps edits made since', () => {
  const before = view(
    [
      { id: 'a', type: 'text', text: 'a', x: 0, y: 0, width: 100, height: 100 },
      { id: 'b', type: 'text', text: 'b', x: 200, y: 0, width: 100, height: 100 },
    ],
    [{ id: 'ab', fromNode: 'a', toNode: 'b' }],
  )
  const change = { nodes: [{ id: 'a', x: 40, label: 'moved' }, { id: 'c', type: 'text', x: 0, y: 300 }], remove: ['b'] }
  const undo = invertPatch(before, change)
  const after = applyPatch(before, change)
  const byId = (board) => Object.fromEntries([...board.nodes, ...board.edges].map((item) => [item.id, item]))
  expect(byId(applyPatch(after, undo))).toEqual(byId(before))
  const agentEdited = applyPatch(after, { nodes: [{ id: 'a', text: 'agent' }] })
  expect(byId(applyPatch(agentEdited, undo)).a).toEqual({ ...before.nodes[0], text: 'agent' })
})

test('copies get fresh ids and keep the connectors among them', () => {
  const nodes = [
    { id: 'a', type: 'text', x: 0, y: 0, width: 10, height: 10 },
    { id: 'b', type: 'group', x: 50, y: 0, width: 10, height: 10 },
  ]
  const copy = cloneItems({ nodes, edges: [{ id: 'ab', fromNode: 'a', toNode: 'b', label: 'next' }] }, 24, 24)
  const ids = copy.nodes.map((node) => node.id)
  expect(ids).not.toContain('a')
  expect(ids).not.toContain('b')
  expect(copy.nodes.map((node) => [node.type, node.x, node.y])).toEqual([['text', 24, 24], ['group', 74, 24]])
  expect(copy.edges).toEqual([{ id: copy.edges[0].id, fromNode: ids[0], toNode: ids[1], label: 'next' }])
  expect(copy.edges[0].id).not.toBe('ab')
})

test('a connector is a curve with an arrow at its far end unless its fields say otherwise', () => {
  const nodes = [{ id: 'a', type: 'text', x: 0, y: 0, width: 100, height: 40 }, { id: 'b', type: 'text', x: 300, y: 0, width: 100, height: 40 }]
  const [plain, styled, odd] = toFlowEdges(view(nodes, [
    { id: 'plain', fromNode: 'a', toNode: 'b' },
    { id: 'styled', fromNode: 'a', toNode: 'b', route: 'elbow', dash: 'dashed', color: 'blue', fromEnd: 'arrow', toEnd: 'circle' },
    { id: 'odd', fromNode: 'a', toNode: 'b', route: 'zigzag', dash: 7, toEnd: 'none', color: '#123456' },
  ]))
  expect(plain.data.look).toEqual({ route: 'curve', dash: 'solid', ink: undefined, fromEnd: 'none', toEnd: 'arrow' })
  expect([plain.markerStart, plain.markerEnd.color]).toEqual([undefined, 'var(--canvas-edge)'])
  expect(styled.data.look).toEqual({ route: 'elbow', dash: 'dashed', ink: { name: 'blue' }, fromEnd: 'arrow', toEnd: 'circle' })
  expect([styled.markerStart.color, styled.markerEnd]).toEqual(['#2f80ed', undefined])
  expect(odd.data.look).toEqual({ route: 'curve', dash: 'solid', ink: { custom: '#123456' }, fromEnd: 'none', toEnd: 'none' })
  expect(odd.markerEnd).toBeUndefined()
})

test('the next part goes a gap from its side, along it past what already stands there', () => {
  const from = { x: 0, y: 0, width: 200, height: 120 }
  expect(nextTo(from, 'top', [])).toEqual({ x: 0, y: -200, width: 200, height: 120 })
  expect(nextTo(from, 'left', [{ x: 300, y: 0, width: 50, height: 50 }])).toEqual({ x: -280, y: 0, width: 200, height: 120 })
  expect(nextTo(from, 'right', [{ x: 400, y: 100, width: 50, height: 50 }, { x: 280, y: 160, width: 200, height: 120 }])).toEqual({ x: 280, y: 320, width: 200, height: 120 })
  expect(nextTo(from, 'bottom', [{ x: 0, y: 200, width: 200, height: 120 }])).toEqual({ x: 240, y: 200, width: 200, height: 120 })
})
