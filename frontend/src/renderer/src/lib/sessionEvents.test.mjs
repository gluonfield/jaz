import { describe, expect, test } from 'bun:test'
import { coalesceSessionEvents, mergeSessionEvent } from './sessionEvents'

function text(seq, content) {
  return {
    seq,
    session_id: 'thread',
    type: 'acp_message',
    content,
    acp: {
      id: 'agent',
      slug: 'agent',
      agent: 'codex',
      session_id: 'agent-session',
      state: 'running',
      text_run_id: 'message:one',
    },
    at: new Date(seq * 1000).toISOString(),
  }
}

function status(seq, state, extra = {}) {
  return {
    ...text(seq, ''),
    type: 'acp',
    acp: { ...text(seq, '').acp, state, ...extra },
  }
}

describe('session event text projection', () => {
  test('appends live text deltas in one run', () => {
    const events = mergeSessionEvent([text(1, 'Hel')], text(2, 'lo'))

    expect(events).toHaveLength(1)
    expect(events[0].content).toBe('Hello')
    expect(events[0].seq).toBe(2)
  })

  test('keeps equal run ids across a terminal status separate', () => {
    const events = mergeSessionEvent([text(1, 'before'), status(2, 'idle')], text(3, 'after'))

    expect(events.map((event) => event.content)).toEqual(['before', '', 'after'])
  })

  test('merges replay deltas across coalesced tool updates', () => {
    const tool = (seq, toolStatus) => status(seq, 'running', {
      tool_calls: [{ id: 'tool-1', title: 'Read', status: toolStatus }],
    })
    const events = coalesceSessionEvents([
      text(1, 'Hel'),
      { ...tool(2, 'pending'), type: 'acp_tool' },
      { ...tool(3, 'completed'), type: 'acp_tool' },
      text(4, 'lo'),
    ])

    expect(events).toHaveLength(2)
    expect(events[0].acp.tool_calls[0].status).toBe('completed')
    expect(events[1].content).toBe('Hello')
  })

  test('does not merge replay text across a task surface', () => {
    const events = coalesceSessionEvents([
      text(1, 'before'),
      status(2, 'running', { plan: [{ content: 'Inspect', status: 'completed' }] }),
      text(3, 'after'),
    ])

    expect(events.map((event) => event.content)).toEqual(['before', '', 'after'])
  })
})
