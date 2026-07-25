import { expect, test } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import Markdown from 'react-markdown'
import { nextStreamTail, rehypeStreamTail } from './streamReveal'

const stream = (...chunks) => {
  let text = ''
  let tail = { text: '', offset: 0, phase: 'a' }
  for (const chunk of chunks) {
    text += chunk
    tail = nextStreamTail(tail, text)
  }
  return renderToStaticMarkup(
    createElement(Markdown, { rehypePlugins: [[rehypeStreamTail, tail]], children: text }),
  )
}

test('an appended chunk reveals only the delta and alternates phase', () => {
  const first = nextStreamTail({ text: '', offset: 0, phase: 'a' }, 'Streaming text')
  const second = nextStreamTail(first, 'Streaming text arrives in chunks')

  expect(first.offset).toBe('Streaming text'.length)
  expect(second.offset).toBe('Streaming text'.length)
  expect(second.phase).not.toBe(first.phase)
})

test('replaced text reveals nothing', () => {
  const prev = { text: 'live draft', offset: 4, phase: 'a' }

  expect(nextStreamTail(prev, 'persisted answer').offset).toBe('persisted answer'.length)
  expect(nextStreamTail(prev, 'live').offset).toBe('live'.length)
})

test('markup in the new chunk does not drag the reveal over settled text', () => {
  const html = stream('Streaming lands in **chunks**', ', and `code()` stays whole.')

  expect(html).toContain('<strong>chunks</strong>')
  expect(html).toContain('<span data-stream-tail="a">, and </span>')
})

test('code keeps its own text and the prose after it still reveals', () => {
  const html = stream('Run it.', ' Use `bun test` next.')

  expect(html).toContain('<code>bun test</code>')
  expect(html).toContain('<span data-stream-tail="a"> next.</span>')
})

test('a settled message reveals nothing', () => {
  expect(stream('Nothing streamed here.')).not.toContain('data-stream-tail')
})
