import { describe, expect, test } from 'bun:test'
import { buildTimeline } from './timeline'

const acpEvent = (id, type, at, fields = {}) => ({
  session_id: 'thread',
  type,
  at: new Date(at * 1000).toISOString(),
  acp: {
    id,
    agent: id,
    session_id: id,
    state: 'running',
    ...fields,
  },
})

describe('ACP activity timeline', () => {
  test('groups alternating reasoning and tools without moving commentary', () => {
    const events = [
      acpEvent('thread', 'acp_thought', 1, { thought: 'inspect files' }),
      acpEvent('thread', 'acp_tool', 2, {
        tool_calls: [{ id: 'tool-2', tool_name: 'exec_command' }],
      }),
      acpEvent('thread', 'acp_thought', 3, { thought: 'compare results' }),
      acpEvent('thread', 'acp_message', 4, {
        assistant: 'The implementation has one important seam.',
      }),
      acpEvent('thread', 'acp_tool', 5, {
        tool_calls: [{ id: 'tool-5', tool_name: 'read' }],
      }),
    ]
    events[3].content = events[3].acp.assistant

    const { turns } = buildTimeline([], events, 'thread', true)
    const items = turns[0].items

    expect(items.map((item) => item.kind)).toEqual(['activity', 'event', 'activity'])
    expect(items[0].entries.map((entry) => entry.kind)).toEqual(['thought', 'tool', 'thought'])
    expect(items[0].entries.map((entry) =>
      entry.kind === 'thought' ? entry.text : entry.call.id,
    )).toEqual(['inspect files', 'tool-2', 'compare results'])
    expect(items[1].event.content).toBe('The implementation has one important seam.')
    expect(items[2].entries[0].call.id).toBe('tool-5')
  })

  test('preserves ACP identity transitions as separate headed activities', () => {
    const events = [
      acpEvent('thread', 'acp_thought', 1, { thought: 'parent reasoning' }),
      acpEvent('other-agent', 'acp_thought', 2, {
        title: 'Independent review',
        thought: 'other reasoning',
      }),
    ]

    const { turns } = buildTimeline([], events, 'thread', true)
    const activities = turns[0].items

    expect(activities.map((item) => item.kind)).toEqual(['activity', 'activity'])
    expect(activities[0].header).toBeUndefined()
    expect(activities[1].header.agent).toBe('other-agent')
    expect(activities[1].header.title).toBe('Independent review')
  })

  test('keeps content-bearing legacy ACP snapshots on the main transcript axis', () => {
    const snapshot = acpEvent('thread', 'acp', 1, {
      assistant: 'Visible assistant text',
      thought: 'reasoning carried by the same snapshot',
      tool_calls: [{ id: 'tool', tool_name: 'read' }],
    })
    snapshot.content = snapshot.acp.assistant

    const { chronological } = buildTimeline([], [snapshot], 'thread', true)

    expect(chronological).toHaveLength(1)
    expect(chronological[0].kind).toBe('event')
    expect(chronological[0].event).toBe(snapshot)
  })

  test('updates one logical tool call without discarding intervening reasoning', () => {
    const events = [
      acpEvent('thread', 'acp_tool', 1, {
        tool_calls: [{ id: 'command', tool_name: 'exec_command', status: 'running' }],
      }),
      acpEvent('thread', 'acp_thought', 2, { thought: 'checking command output' }),
      acpEvent('thread', 'acp_tool', 3, {
        tool_calls: [{ id: 'command', tool_name: 'exec_command', status: 'completed' }],
      }),
    ]

    const { chronological } = buildTimeline([], events, 'thread', true)
    const entries = chronological[0].entries

    expect(entries).toHaveLength(2)
    expect(entries[0].key).toBe('tool-thread:command')
    expect(entries[0].call.status).toBe('completed')
    expect(entries[1].text).toBe('checking command output')
  })

  test('gives separate activity runs unique keys when one tool resumes after commentary', () => {
    const events = [
      acpEvent('thread', 'acp_tool', 1, {
        tool_calls: [{ id: 'command', tool_name: 'exec_command', status: 'running' }],
      }),
      { ...acpEvent('thread', 'acp_message', 2), content: 'visible commentary' },
      acpEvent('thread', 'acp_tool', 3, {
        tool_calls: [{ id: 'command', tool_name: 'exec_command', status: 'completed' }],
      }),
    ]

    const { chronological } = buildTimeline([], events, 'thread', true)

    expect(chronological.map((item) => item.kind)).toEqual(['activity', 'event', 'activity'])
    expect(chronological[0].key).not.toBe(chronological[2].key)
  })
})
