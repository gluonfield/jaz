import { describe, expect, test } from 'bun:test'
import { hasCompletedLiveReply } from './liveTranscript'

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
  assistant: '',
  tools: [],
})

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
})
