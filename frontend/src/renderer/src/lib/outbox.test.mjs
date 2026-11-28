import { describe, expect, test } from 'bun:test'
import { outbox, outboxTranscript, patchOutboxMessage, putOutboxMessage, removeOutboxMessages } from './outbox'

const message = (seq, role, content) => ({
  seq,
  role,
  from_user: role === 'user',
  content,
  blocks: [{ type: 'text', text: content }],
  created_at: new Date(seq * 1000).toISOString(),
})

const entry = (id, content, baselineMessageSeq, state = 'sent') => ({
  id,
  state,
  baselineMessageSeq,
  message: {
    role: 'user',
    content,
    blocks: [{ type: 'text', text: content }],
    created_at: new Date((baselineMessageSeq + 0.5) * 1000).toISOString(),
  },
})

const shown = (messages, entries) => outboxTranscript(messages, entries).messages.map((item) => item.content)

describe('outbox transcript', () => {
  test('a message stays at the end until its saved row loads, then stands in that row', () => {
    const ready = message(1, 'assistant', 'ready')
    const sending = outboxTranscript([ready], [entry('a', 'hi', 1, 'sending')]).messages
    expect(sending.map((item) => item.content)).toEqual(['ready', 'hi'])
    expect(sending[1].delivery).toEqual({ id: 'a', state: 'sending', error: undefined })

    const accepted = outboxTranscript([ready], [entry('a', 'hi', 1)]).messages
    expect(accepted.map((item) => item.content)).toEqual(['ready', 'hi'])
    expect(accepted[1].delivery).toBeUndefined()

    const saved = outboxTranscript(
      [ready, message(2, 'user', 'hi'), message(3, 'assistant', 'hello')],
      [entry('a', 'hi', 1)],
    ).messages
    expect(saved.map((item) => [item.seq, item.content])).toEqual([[1, 'ready'], [2, 'hi'], [3, 'hello']])
    expect(saved[1].created_at).toBe(new Date(1500).toISOString())
  })

  // The chat used to hold one unsaved message: a second send took its place,
  // and the first came back only once both were saved.
  test('a second send never hides the first, whatever history has loaded', () => {
    const earlier = message(10, 'assistant', 'working on it')
    const first = entry('a', 'do the report', 10)
    const second = entry('b', 'and email it', 10, 'sending')
    const both = ['working on it', 'do the report', 'and email it']

    expect(shown([earlier], [{ ...first, state: 'sending' }, second])).toEqual(both)
    expect(shown([earlier], [first, second])).toEqual(both)
    expect(shown([earlier, message(11, 'user', 'do the report')], [first, second])).toEqual(both)
    expect(shown([earlier, message(11, 'user', 'do the report')], [first, { ...second, state: 'sent' }])).toEqual(both)
    expect(shown(
      [earlier, message(11, 'user', 'do the report'), message(12, 'user', 'and email it')],
      [first, { ...second, state: 'sent' }],
    )).toEqual(both)
  })

  test('an accepted message outlives a history reload that does not hold it yet', () => {
    const stale = [message(1, 'assistant', 'ready')]
    const delivered = entry('a', 'hi', 1, 'delivered')
    expect(outboxTranscript(stale, [delivered])).toEqual({
      messages: [stale[0], { ...delivered.message, seq: 1_000_001, from_user: true }],
      settled: [],
    })
    const loaded = [...stale, message(2, 'user', 'hi')]
    expect(outboxTranscript(loaded, [delivered])).toEqual({ messages: loaded, settled: ['a'] })
  })

  test('only a row of the same text saved after the send can be its row', () => {
    const internal = { ...message(2, 'user', 'Keep working'), from_user: false }
    expect(shown([message(1, 'user', 'hi'), message(2, 'assistant', 'hello')], [entry('a', 'hi', 2)])).toEqual(['hi', 'hello', 'hi'])
    expect(shown([internal], [entry('a', 'Keep working', 1)])).toEqual(['Keep working', 'Keep working'])
    expect(shown([message(2, 'user', 'from my phone')], [entry('a', 'hi', 1)])).toEqual(['from my phone', 'hi'])
    expect(outboxTranscript([internal, message(3, 'user', 'Keep working')], [entry('a', 'Keep working', 1)]).messages[0]).toBe(internal)
  })

  test('the same text sent twice takes two rows', () => {
    const entries = [entry('a', 'hi', 1, 'delivered'), entry('b', 'hi', 1, 'delivered')]
    expect(outboxTranscript([message(2, 'user', 'hi')], entries).settled).toEqual(['a'])
    expect(shown([message(2, 'user', 'hi')], entries)).toEqual(['hi', 'hi'])
    expect(outboxTranscript([message(2, 'user', 'hi'), message(3, 'user', 'hi')], entries).settled).toEqual(['a', 'b'])
  })

  test('a refused message stays, marked, and takes no row', () => {
    const failed = { ...entry('a', 'hi', 1, 'failed'), error: 'agent did not start' }
    const result = outboxTranscript([message(2, 'user', 'hi')], [failed])
    expect(result.settled).toEqual([])
    expect(result.messages.map((item) => item.delivery)).toEqual([
      undefined,
      { id: 'a', state: 'failed', error: 'agent did not start' },
    ])
  })
})

test('an outbox is kept per thread, apart from any view of it', () => {
  const send = { ...entry('a', 'hi', 1, 'sending'), text: 'hi', options: {}, uploaded: [] }
  putOutboxMessage('thread', send)
  putOutboxMessage('thread', { ...send, id: 'b' })
  patchOutboxMessage('thread', 'a', { state: 'failed', error: 'offline' })
  patchOutboxMessage('thread', 'gone', { state: 'sent' })
  expect(outbox('thread').map((item) => [item.id, item.state])).toEqual([['a', 'failed'], ['b', 'sending']])
  expect(outbox('other')).toEqual([])
  putOutboxMessage('thread', { ...send, state: 'sending' })
  expect(outbox('thread').map((item) => [item.id, item.state])).toEqual([['a', 'sending'], ['b', 'sending']])
  removeOutboxMessages('thread', ['a', 'b'])
  expect(outbox('thread')).toEqual([])
})
