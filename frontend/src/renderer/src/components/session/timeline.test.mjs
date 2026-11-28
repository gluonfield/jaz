import { describe, expect, test } from 'bun:test'
import { buildTimeline, classifyTurnItems, stableEventKey } from './timeline'

test('batch lookup apps stay off the timeline while explicit opens and tool history remain', () => {
  const at = (second) => new Date(second * 1000).toISOString()
  const app = (seq, tool, server_id = 'crm') => ({
    session_id: 'thread', type: 'mcp_app', seq, at: at(seq),
    mcp_app: { server_id, tool, arguments: {}, result: { content: [], structuredContent: { id: String(seq) } } },
  })
  const lookup = { id: 'lookup', title: 'get_record', status: 'completed' }
  const work = acpEvent('thread', 'acp', 1, { tool_calls: [lookup] })
  const opened = app(505, 'show_crm')
  const selected = app(508, 'show_issues', 'tasks')
  selected.mcp_app.presented = true
  selected.mcp_app.result.content.push({ type: 'resource_link', uri: 'ui://tasks/issue', name: 'Tasks', mimeType: 'text/html;profile=mcp-app' })
  const answer = { session_id: 'thread', type: 'acp_message', seq: 507, at: at(507), content: 'All records verified.' }
  const events = [
    work,
    ...Array.from({ length: 503 }, (_, i) => app(i + 2, i < 8 ? 'search_records' : 'get_record')),
    opened,
    app(506, 'show_crm', 'different-server'),
    answer,
    selected,
  ]
  const entrypoints = [{ server_id: 'crm', tool: 'show_crm', type: 'thread', title: 'Customers' }]
  for (const grouped of [false, true]) {
    for (const catalog of [[], entrypoints]) {
      const timeline = buildTimeline([], events, 'thread', grouped, catalog)
      expect(timeline.chronological[0].entries[0].call).toBe(lookup)
      expect(timeline.chronological.filter((item) => item.kind === 'event').map((item) => item.event))
        .toEqual(catalog.length ? [opened, answer, selected] : [answer, selected])
    }
  }
  expect(events.filter((event) => event.type === 'mcp_app')).toHaveLength(506)
})

test('questions stay inline and visible before and after resolution while approvals remain anchored', () => {
  const at = (second) => new Date(second * 1000).toISOString()
  const request = {
    session_id: 'thread', type: 'permission_request', at: at(2),
    permission: { id: 'question', status: 'pending', questions: [{ id: 'scope', question: 'Which workloads?' }] },
  }
  const commentary = { session_id: 'thread', type: 'acp_message', at: at(3), content: 'I can start with SQL.' }
  const resolution = {
    session_id: 'thread', type: 'permission_response', at: at(4),
    permission: { ...request.permission, status: 'selected', answers: { scope: ['SQL'] } },
  }
  for (const events of [[request, commentary], [request, commentary, resolution]]) {
    for (const grouped of [false, true]) {
      const timeline = buildTimeline([], events, 'thread', grouped)
      expect(timeline.anchored).toEqual([])
      expect(timeline.chronological.map((item) => item.event)).toEqual([request, commentary])
      const classified = classifyTurnItems(timeline.chronological, timeline.pendingPermissionIds, new Map())
      expect(classified.resultItems.some((item) => item.event === request)).toBe(true)
      expect(timeline.permissionResolutions.get('question')).toEqual(events.at(-1) === resolution ? resolution.permission : undefined)
    }
  }
  const approval = { ...request, permission: { id: 'approval', status: 'pending', options: [{ id: 'allow', name: 'Allow' }] } }
  expect(buildTimeline([], [approval], 'thread', true).anchored[0].event).toEqual(approval)
})

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

test('speech forms normal turns while typed handoff context stays off the reading axis', () => {
  const at = (second) => new Date(second * 1000).toISOString()
  const speech = (id, role, text, second) => ({
    type: 'voice_message', session_id: 'thread', seq: second + 100, at: at(100),
    projection_key: 'voice:thread:call:' + id, projection_op: 'replace',
    voice: { id, call_id: 'call', role, text, at: at(second) },
  })
  const messages = [
    { seq: 1, role: 'user', content: 'List the files', blocks: [{ type: 'voice_context', id: 'call', text: 'User: Hello\nVoice: Hi.' }], created_at: at(3) },
    { seq: 2, role: 'user', content: 'A typed follow-up', blocks: [], created_at: at(9) },
  ]
  const answer = { ...acpEvent('thread', 'acp_message', 5), content: 'Files: **README.md**, backend/, frontend/.' }
  const user = speech('user', 'user', 'What files are here?', 1)
  const spoken = speech('reply', 'assistant', 'I’ll check the directory.', 2)
  const events = [answer, spoken, user]
  const result = buildTimeline(messages, events, 'thread', true)
  expect(result.turns).toHaveLength(2)
  expect(result.turns[0].opener.event.voice.text).toBe('What files are here?')
  expect(result.turns[0].items.map((item) => item.event)).toEqual([spoken, answer])
  expect(result.turns[1].opener.message.content).toBe('A typed follow-up')
  const classified = classifyTurnItems(result.turns[0].items, new Set(), new Map())
  expect(classified.resultItems.map((item) => item.event)).toEqual([spoken, answer])
  expect(result.turns[0].items[0].collapseVoice).toBe(true)
  expect(stableEventKey(user)).toBe(stableEventKey({ ...user, seq: 999 }))
  expect(buildTimeline(messages, [answer], 'thread', true).turns[0].opener.message.content).toBe('List the files')
})

test('spoken copies are expandable beside written replies without changing either transcript', () => {
  const written = { ...acpEvent('thread', 'acp_message', 2), content: 'Dogtooth, Shadow Robot and Dexory stand out because published supplier cases confirm they buy parts externally.' }
  const spoken = {
    session_id: 'thread', type: 'voice_message', at: new Date(3000).toISOString(),
    voice: { id: 'reply', call_id: 'call', role: 'assistant', text: 'Dogtooth, Shadow Robot, and Dexory stand out because published supplier cases confirm they buy parts externally.', at: new Date(3000).toISOString() },
  }
  const before = globalThis.structuredClone([written, spoken])
  for (const groupTurns of [true, false]) {
    const result = buildTimeline([], [written, spoken], 'thread', groupTurns)
    expect(result.chronological.map((item) => item.event)).toEqual(before)
    expect(result.chronological[1].collapseVoice).toBe(true)
  }
  expect([written, spoken]).toEqual(before)
  expect(buildTimeline([], [spoken], 'thread', true).chronological[0].collapseVoice).toBeUndefined()
})

test('voice-only turns stay visible after earlier written answers and child output', () => {
  const at = (second) => new Date(second * 1000).toISOString()
  const speech = (role, second) => ({
    session_id: 'thread', type: 'voice_message', at: at(second),
    voice: { id: String(second), call_id: 'call', role, text: role === 'user' ? 'Thanks.' : 'You’re welcome.', at: at(second) },
  })
  const events = [
    { ...acpEvent('thread', 'acp_message', 1), content: 'Earlier written answer' },
    speech('user', 2),
    { ...acpEvent('child', 'acp_message', 3), content: 'Child output' },
    speech('assistant', 4),
  ]
  const result = buildTimeline([], events, 'thread', true)
  expect(result.turns[1].opener.collapseVoice).toBeUndefined()
  expect(result.turns[1].items.at(-1).collapseVoice).toBeUndefined()
  const messages = [{ seq: 1, role: 'assistant', content: 'Written response', created_at: at(3) }]
  expect(buildTimeline(messages, [events[1], events[3]], 'thread', true).turns[0].items.at(-1).collapseVoice).toBe(true)
})

test('only readable assistant text can collapse a spoken answer', () => {
  const at = new Date(1000).toISOString()
  const spoken = {
    session_id: 'thread', type: 'voice_message', at,
    voice: { id: 'reply', call_id: 'call', role: 'assistant', text: 'Your disk has 42 GB free.', at },
  }
  const tool = { type: 'tool', id: 'disk', name: 'exec_command', result: '42 GB' }
  const thought = { type: 'reasoning', text: 'Check disk space.' }
  const message = { seq: 1, role: 'assistant', content: '', blocks: [thought, tool], created_at: at }
  for (const blocks of [[], [thought], [tool], [thought, tool]]) {
    const timeline = buildTimeline([{ ...message, blocks }], [spoken], 'thread', true)
    expect(timeline.chronological.at(-1).collapseVoice).toBeUndefined()
  }
  const answer = { ...message, blocks: [...message.blocks, { type: 'text', text: '42 GB free.' }] }
  expect(buildTimeline([answer], [spoken], 'thread', true).chronological.at(-1).collapseVoice).toBe(true)
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

test('a saved app stands outside the folded work once per call, and a failed save stays inside it', () => {
  const saved = JSON.stringify({ app_id: 'a'.repeat(26), title: 'World Clock', kind: 'artifact', version: 1 })
  const read = { id: 'read', title: 'Read file', status: 'completed' }
  const failed = { id: 'failed', tool_name: 'publish_app', status: 'failed', raw_output: { error: 'missing entry' } }
  const running = { id: 'save', tool_name: 'publish_app', status: 'in_progress' }
  const done = { ...running, status: 'completed', raw_output: saved }
  const user = (seq, second) => ({ seq, role: 'user', content: 'Save it', created_at: new Date(second * 1000).toISOString() })
  const timeline = buildTimeline(
    [user(1, 0)],
    [
      acpEvent('thread', 'acp_tool', 1, { tool_calls: [read, failed, running] }),
      { ...acpEvent('thread', 'acp_message', 2), content: 'Saving the clock now.' },
      acpEvent('thread', 'acp_tool', 3, { tool_calls: [done] }),
      { ...acpEvent('thread', 'acp_message', 4), content: 'Saved.' },
    ].map((event, index) => ({ ...event, seq: index + 1 })),
    'thread',
    true,
  )
  const apps = timeline.chronological.filter((item) => item.kind === 'app')
  expect(apps.map((item) => item.call)).toEqual([done])

  const { workItems, resultItems } = classifyTurnItems(timeline.turns[0].items, new Set(), new Map())
  expect(workItems.flatMap((item) => item.entries?.map((entry) => entry.call.id) ?? [])).toEqual(['read', 'failed'])
  expect(resultItems.map((item) => item.kind === 'app' ? 'app' : item.event.content)).toEqual(['Saving the clock now.', 'app', 'Saved.'])
})

test('user and turn lifecycle events carrying text never render as assistant responses', () => {
  const at = (second) => new Date(second * 1000).toISOString()
  const events = [
    { session_id: 'thread', type: 'user_message', seq: 1, at: at(1), content: 'Hello from user' },
    { session_id: 'thread', type: 'turn_started', seq: 2, at: at(2), content: 'Hello from user' },
    { session_id: 'thread', type: 'turn_queued', seq: 3, at: at(3), content: 'Queued prompt' },
    { session_id: 'thread', type: 'session', seq: 4, at: at(4), content: 'Refresh signal' },
    { session_id: 'thread', type: 'acp_message', seq: 5, at: at(5), content: 'Real assistant reply' },
  ]
  for (const grouped of [false, true]) {
    const timeline = buildTimeline([], events, 'thread', grouped)
    expect(timeline.chronological.map((item) => item.event.content)).toEqual(['Real assistant reply'])
  }
})
