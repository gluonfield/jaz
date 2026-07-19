import { describe, expect, test } from 'bun:test'
import { coalesceSessionEvents, mergeSessionEvent } from './sessionEvents'

function text(seq, content, projectionOp) {
  return {
    seq,
    projection_key: 'acp_text:thread:agent:acp_message:message:one:1',
    projection_op: projectionOp,
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
    const events = mergeSessionEvent([text(1, 'Hel')], text(2, 'lo', 'append'))

    expect(events).toHaveLength(1)
    expect(events[0].content).toBe('Hello')
    expect(events[0].seq).toBe(2)
  })

  test('rejects stale deltas even when their projection key matches', () => {
    const events = mergeSessionEvent([text(2, 'current')], text(1, 'stale', 'append'))

    expect(events).toHaveLength(1)
    expect(events[0].content).toBe('current')
  })

  test('retains an older distinct event during reconnect replay', () => {
    const older = { ...status(1, 'running'), projection_key: 'acp_status:agent' }
    const events = mergeSessionEvent([text(2, 'current')], older)

    expect(events).toHaveLength(2)
    expect(events[1]).toBe(older)
  })

  test('replaces semantic projections using backend identity', () => {
    const tool = (seq, toolStatus) => status(seq, 'running', {
      tool_calls: [{ id: 'tool-1', title: 'Read', status: toolStatus }],
    })
    const events = coalesceSessionEvents([
      { ...tool(1, 'pending'), type: 'acp_tool', projection_key: 'acp_tool:agent:tool-1' },
      {
        ...tool(2, 'completed'),
        type: 'acp_tool',
        projection_key: 'acp_tool:agent:tool-1',
        projection_op: 'replace',
      },
    ])

    expect(events).toHaveLength(1)
    expect(events[0].acp.tool_calls[0].status).toBe('completed')
  })

  test('combines a compact snapshot with later append operations', () => {
    const events = coalesceSessionEvents([text(2, 'Hello'), text(3, ' world', 'append')])

    expect(events).toHaveLength(1)
    expect(events[0].content).toBe('Hello world')
  })
})
