import { expect, test } from 'bun:test'
import { nextStreamTail, rehypeStreamTail } from './streamReveal'

const paragraph = (...children) => ({
  type: 'root',
  children: [{ type: 'element', tagName: 'p', properties: {}, children }],
})
const text = (value) => ({ type: 'text', value })

test('an appended chunk reveals only the delta and alternates phase', () => {
  const first = nextStreamTail({ text: '', chars: 0, phase: 'a' }, 'Streaming text')
  const second = nextStreamTail(first, 'Streaming text arrives in chunks')

  expect(first.chars).toBe(0)
  expect(second.chars).toBe(' arrives in chunks'.length)
  expect(second.phase).not.toBe(first.phase)
})

test('replaced text reveals nothing', () => {
  const prev = { text: 'live draft', chars: 4, phase: 'a' }

  expect(nextStreamTail(prev, 'persisted answer').chars).toBe(0)
  expect(nextStreamTail(prev, 'live').chars).toBe(0)
})

test('the tail pass splits the boundary text node', () => {
  const tree = paragraph(text('settled and new'))
  rehypeStreamTail({ text: '', chars: 7, phase: 'a' })(tree)

  expect(tree.children[0].children).toEqual([
    text('settled '),
    {
      type: 'element',
      tagName: 'span',
      properties: { dataStreamTail: 'a' },
      children: [text('and new')],
    },
  ])
})

test('code spends the reveal budget without being wrapped', () => {
  const tree = paragraph(text('settled prose'), {
    type: 'element',
    tagName: 'code',
    properties: {},
    children: [text('run()')],
  })
  rehypeStreamTail({ text: '', chars: 5, phase: 'a' })(tree)

  expect(tree.children[0].children.map((node) => node.tagName ?? node.value)).toEqual([
    'settled prose',
    'code',
  ])
})
