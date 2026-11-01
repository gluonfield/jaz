import { describe, expect, test } from 'bun:test'
import { optimisticTranscriptMessages } from '@/lib/optimisticUserMessage'
import { hasCompletedLiveReply, liveOptimisticUserMessage } from './liveTranscript'

const message = (seq, role, content) => ({
  seq,
  role,
  from_user: role === 'user',
  content,
  blocks: [{ type: 'text', text: content }],
  created_at: new Date(seq * 1000).toISOString(),
})

const live = (user, baselineMessageSeq) => ({
  user,
  baselineMessageSeq,
  at: new Date((baselineMessageSeq + 0.5) * 1000).toISOString(),
  baselineEventSeq: baselineMessageSeq,
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
  test('a stream error clears only after a post-send assistant reply completes', () => {
    const exchange = { ...live('hi', 2), error: 'network error' }
    const history = {
      session: { id: 'thread', runtime: 'acp', status: 'idle' },
      acp_state: 'idle',
      messages: [message(3, 'assistant', 'hello')],
      events: [],
    }

    expect(hasCompletedLiveReply(exchange, history)).toBe(true)
    expect(hasCompletedLiveReply(exchange, { ...history, acp_state: 'running' })).toBe(false)
    expect(hasCompletedLiveReply(exchange, { ...history, acp_state: undefined })).toBe(false)
    expect(hasCompletedLiveReply(exchange, { ...history, session: { ...history.session, status: 'error' } })).toBe(false)
    expect(hasCompletedLiveReply(exchange, { ...history, messages: [message(2, 'assistant', 'old reply')] })).toBe(false)
  })

  test('a completed ACP reply can confirm recovery without a stored assistant message', () => {
    const exchange = { ...live('hi', 2), baselineEventSeq: 5, error: 'network error' }
    const history = {
      session: { id: 'thread', runtime: 'acp', status: 'idle' },
      acp_state: 'idle',
      messages: [],
      events: [{ session_id: 'thread', seq: 6, type: 'acp_message', content: 'hello', acp: { id: 'thread' } }],
    }

    expect(hasCompletedLiveReply(exchange, history)).toBe(true)
    expect(hasCompletedLiveReply(exchange, { ...history, events: [{ ...history.events[0], seq: 5 }] })).toBe(false)
    expect(hasCompletedLiveReply(exchange, { ...history, events: [{ ...history.events[0], acp: { id: 'child' } }] })).toBe(false)
  })

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

  test('an internal coordination arrival cannot acknowledge or replace a pending human send', () => {
    const internal = { ...message(2, 'user', 'Internal coordination'), from_user: false }
    const result = projected([internal], live('Keep working', 1))
    expect(result.map((item) => item.content)).toEqual(['Internal coordination', 'Keep working'])
    expect(result[1].from_user).toBe(true)
    const saved = message(3, 'user', 'Keep working')
    const accepted = projected([internal, saved], live('Keep working', 1))
    expect(accepted.map((item) => item.content)).toEqual(['Internal coordination', 'Keep working'])
    expect(accepted[0]).toBe(internal)
    expect(accepted[1].from_user).toBe(true)
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
