import { describe, expect, test } from 'bun:test'
import { mergeEarlierHistory, mergeLatestHistory } from '../sessionHistory'

const session = { id: 'thread' }
const message = (seq, content) => ({ seq, role: seq % 2 ? 'user' : 'assistant', content, blocks: [], created_at: new Date(seq * 1000).toISOString() })
const text = (seq, content) => ({
  seq,
  projection_key: 'acp_text:thread:agent:acp_message:message:one',
  projection_op: 'append',
  session_id: 'thread',
  type: 'acp_message',
  content,
  acp: { id: 'agent', text_run_id: 'message:one' },
  at: new Date(seq * 1000).toISOString(),
})

describe('session history ownership', () => {
  test('keeps earlier pages when the latest page refetches', () => {
    const current = {
      session,
      history_revision: 4,
      messages: [message(1, 'old'), message(2, 'middle')],
      events: [text(1, 'Hel')],
      has_earlier: false,
    }
    const latest = {
      session,
      history_revision: 4,
      messages: [message(2, 'middle'), message(3, 'latest')],
      events: [text(2, 'lo')],
      has_earlier: true,
      before_message_seq: 2,
    }

    const merged = mergeLatestHistory(current, latest)

    expect(merged.messages.map((item) => item.seq)).toEqual([1, 2, 3])
    expect(merged.events).toHaveLength(1)
    expect(merged.events[0].content).toBe('Hello')
    expect(merged.has_earlier).toBe(false)
  })

  test('drops stale accumulated pages when compaction changes the revision', () => {
    const current = { session, history_revision: 4, messages: [message(1, 'stale')] }
    const latest = { session, history_revision: 5, messages: [message(9, 'fresh')] }

    expect(mergeLatestHistory(current, latest)).toBe(latest)
  })

  test('composes a text run split across physical pages', () => {
    const current = { session, history_revision: 7, messages: [], events: [text(2, 'lo')] }
    const earlier = { session, history_revision: 7, messages: [], events: [text(1, 'Hel')] }

    const merged = mergeEarlierHistory(current, earlier)

    expect(merged.events).toHaveLength(1)
    expect(merged.events[0].content).toBe('Hello')
  })
})
