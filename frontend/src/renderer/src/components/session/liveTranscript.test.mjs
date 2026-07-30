import { describe, expect, test } from 'bun:test'
import { liveTranscriptMessages } from './liveTranscript'

const message = (seq, role, content) => ({
  seq,
  role,
  content,
  blocks: [{ type: 'text', text: content }],
  created_at: new Date(seq * 1000).toISOString(),
})

const live = (user, baselineMessageSeq) => ({
  user,
  baselineMessageSeq,
  at: new Date((baselineMessageSeq + 0.5) * 1000).toISOString(),
  planRequested: false,
  goalRequested: false,
  contexts: [],
  attachments: [],
  reasoning: '',
  assistant: '',
  tools: [],
})

describe('live transcript projection', () => {
  test('adds the optimistic user message until it is persisted', () => {
    const messages = [message(1, 'assistant', 'ready')]

    expect(liveTranscriptMessages(messages, live('hi', 1), true).map((item) => item.content)).toEqual([
      'ready',
      'hi',
    ])
  })

  test('replaces its persisted message without duplicating it', () => {
    const messages = [
      message(1, 'assistant', 'ready'),
      message(2, 'user', 'hi'),
      message(3, 'assistant', 'hello'),
    ]

    const projected = liveTranscriptMessages(messages, live('hi', 1), true)

    expect(projected.map((item) => item.content)).toEqual(['ready', 'hi', 'hello'])
    expect(projected[1].created_at).toBe(new Date(1500).toISOString())
  })

  test('drops a stale optimistic message after a steered successor is persisted', () => {
    const messages = [
      message(1, 'user', 'hi'),
      message(2, 'user', 'one'),
      message(3, 'user', 'two'),
    ]

    expect(liveTranscriptMessages(messages, live('hi', 0), true)).toBe(messages)
  })

  test('does not mistake the same text from before the send baseline for its message', () => {
    const messages = [
      message(1, 'user', 'hi'),
      message(2, 'assistant', 'hello'),
    ]

    expect(liveTranscriptMessages(messages, live('hi', 2), true).map((item) => item.content)).toEqual([
      'hi',
      'hello',
      'hi',
    ])
  })
})
