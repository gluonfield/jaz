import { describe, expect, test } from 'bun:test'
import { optimisticTranscriptMessages } from '@/lib/optimisticUserMessage'
import { liveOptimisticUserMessage } from './liveTranscript'

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

const projected = (messages, exchange) =>
  optimisticTranscriptMessages(messages, liveOptimisticUserMessage(exchange))

describe('live transcript projection', () => {
  test('adds the optimistic user message until it is persisted', () => {
    const messages = [message(1, 'assistant', 'ready')]

    expect(projected(messages, live('hi', 1)).map((item) => item.content)).toEqual([
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

    const result = projected(messages, live('hi', 1))

    expect(result.map((item) => item.content)).toEqual(['ready', 'hi', 'hello'])
    expect(result[1].created_at).toBe(new Date(1500).toISOString())
  })

  test('drops a stale optimistic message after a steered successor is persisted', () => {
    const messages = [
      message(1, 'user', 'hi'),
      message(2, 'user', 'one'),
      message(3, 'user', 'two'),
    ]

    expect(projected(messages, live('hi', 0))).toBe(messages)
  })

  test('does not mistake the same text from before the send baseline for its message', () => {
    const messages = [
      message(1, 'user', 'hi'),
      message(2, 'assistant', 'hello'),
    ]

    expect(projected(messages, live('hi', 2)).map((item) => item.content)).toEqual([
      'hi',
      'hello',
      'hi',
    ])
  })
})
