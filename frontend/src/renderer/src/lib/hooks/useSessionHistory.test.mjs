import { describe, expect, test } from 'bun:test'
import { loadCompleteHistoryBatch, mergeEarlierHistory, mergeLatestHistory } from '../sessionHistory'

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

  test('completes the newest turn across physical event pages before exposing older history', async () => {
    const current = {
      session,
      history_revision: 7,
      messages: [message(15, 'latest prompt')],
      events: [text(600, 'end')],
      has_earlier: true,
      before_message_seq: 15,
      before_event_seq: 500,
    }
    const pages = [
      {
        ...current,
        messages: [],
        events: [text(499, 'middle')],
        before_event_seq: 250,
      },
      {
        ...current,
        messages: [],
        events: [text(249, 'start')],
        before_event_seq: undefined,
      },
      {
        ...current,
        messages: [message(1, 'earlier prompt')],
        events: [],
        has_earlier: false,
        before_message_seq: undefined,
        before_event_seq: undefined,
      },
    ]
    let calls = 0

    const loaded = await loadCompleteHistoryBatch(current, async () => pages[calls++])

    expect(calls).toBe(3)
    expect(loaded.messages.map((item) => item.seq)).toEqual([1, 15])
    expect(loaded.events[0].content).toBe('startmiddleend')
  })

  test('stops a bot thread at the page where a turn opens instead of loading its whole history', async () => {
    const room = (seq, speaker) => ({ seq, session_id: 'bot', type: 'room_message', room_message: { speaker, text: `${speaker} ${seq}` }, at: new Date(seq * 1000).toISOString() })
    const current = { session, history_revision: 3, messages: [], events: [room(900, 'bot')], has_earlier: true, before_event_seq: 900 }
    const pages = [
      { ...current, events: [room(600, 'bot')], before_event_seq: 600 },
      { ...current, events: [room(300, 'user'), room(400, 'bot')], before_event_seq: 300 },
      { ...current, events: [room(100, 'user')], has_earlier: false, before_event_seq: undefined },
    ]
    let calls = 0

    const loaded = await loadCompleteHistoryBatch(current, async () => pages[calls++])

    expect(calls).toBe(2)
    expect(loaded.events.map((event) => event.seq)).toEqual([300, 400, 600, 900])
    expect(loaded.has_earlier).toBe(true)
  })

  test('rejects a continuation page whose cursors do not advance', async () => {
    const current = {
      session,
      history_revision: 7,
      messages: [message(15, 'latest prompt')],
      events: [],
      has_earlier: true,
      before_message_seq: 15,
    }

    expect(loadCompleteHistoryBatch(current, async () => current)).rejects.toThrow(
      'Earlier history cursor did not advance',
    )
  })
})
