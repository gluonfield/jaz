import { expect, test } from 'bun:test'
import { deriveSessionView } from '@/components/session/sessionView'
import { findActiveTrigger } from '@/components/session/composerTokens'
import { mergeSessionEvent } from '@/lib/sessionEvents'

test('native commands autocomplete only at the start and retain ordinary arguments', () => {
  expect(findActiveTrigger('/comp', 5)).toEqual({ trigger: '/', start: 0, query: 'comp' })
  expect(findActiveTrigger('/compact preserve context', 25)).toBeNull()
  expect(findActiveTrigger('see /tmp', 8)).toBeNull()
})

test('native controls survive history pagination and clear when the provider withdraws them', () => {
  const state = { config_options: [{ id: 'fast-mode', current_value: 'on' }], commands: [{ name: 'compact' }] }
  const overview = { agent_events: [event(2, 'agent_session', { agent_session: state })] }
  expect(deriveSessionView(data(), [], overview).agentSession).toEqual(state)
  const cleared = { config_options: [], commands: [] }
  expect(deriveSessionView(data(), [event(3, 'agent_session', { agent_session: cleared })], overview).agentSession).toEqual(cleared)
})

const at = (seconds) => new Date(seconds * 1000).toISOString()
const event = (seq, type, fields = {}) => ({
  session_id: 'thread',
  seq,
  type,
  at: at(seq),
  acp: { id: 'thread', agent: 'codex', state: 'running' },
  ...fields,
})
const data = (events = [], userAt = 1) => ({
  session: { id: 'thread', runtime: 'acp', status: 'running', updated_at: at(1) },
  messages: [{ seq: 1, role: 'user', content: 'Prompt', blocks: [], created_at: at(userAt) }],
  events,
})

test('thinking follows the latest own ACP activity across persisted and streamed updates', () => {
  const events = [
    event(2, 'acp_thought', { acp: { id: 'thread', thought: 'Inspect the code' } }),
    event(3, 'acp_tool', { acp: { id: 'thread', tool_calls: [{ id: 'read', status: 'in_progress' }] } }),
    event(4, 'acp_thought', { acp: { id: 'thread', thought: 'Compare the results' } }),
    event(5, 'acp_message', { content: 'Here is the result.' }),
  ]
  const expected = [false, true, false, true, false]
  for (let count = 0; count <= events.length; count += 1) {
    const current = events.slice(0, count)
    const activity = expected[count] ? 'thinking' : 'working'
    expect(deriveSessionView(data(current), []).activity).toBe(activity)
    expect(deriveSessionView(data(current.slice(0, -1)), current.slice(-1)).activity).toBe(activity)
  }
})

test('thought updates keep their signal after coalescing and ignore metadata and other agents', () => {
  const first = event(2, 'acp_thought', {
    projection_key: 'thought-1',
    projection_op: 'append',
    acp: { id: 'thread', thought: 'Inspect' },
  })
  const tool = event(3, 'acp_tool')
  const next = { ...first, seq: 4, at: at(4), acp: { id: 'thread', thought: ' the result' } }
  const unrelated = [
    event(5, 'acp', { acp: { id: 'thread', state: 'running' } }),
    event(6, 'acp_tool', { acp: { id: 'child', parent_id: 'thread' } }),
    event(7, 'provider_subagent'),
  ]
  expect(deriveSessionView(data([first, tool]), [next, ...unrelated]).activity).toBe('thinking')
})

test('a new user turn or an aggregate snapshot cannot inherit an old thinking signal', () => {
  const old = event(2, 'acp_thought', { acp: { id: 'thread', thought: 'Old reasoning' } })
  expect(deriveSessionView(data([old], 3), []).activity).toBe('working')
  expect(deriveSessionView({ ...data(), acp_thought: 'Unknown order' }, []).activity).toBe('working')
})

test('a starting agent stays starting until it streams its own first event', () => {
  const starting = { ...data(), acp_state: 'starting' }
  expect(deriveSessionView(starting, []).activity).toBe('starting')
  expect(deriveSessionView(starting, [event(3, 'acp_tool', { acp: { id: 'child', parent_id: 'thread' } })]).activity).toBe('starting')
  expect(deriveSessionView(starting, [event(3, 'acp')]).activity).toBe('working')
  expect(deriveSessionView({ ...data(), acp_state: 'running' }, []).activity).toBe('working')
})

test('clearing goal mode stays off when streamed status replaces an earlier cache entry', () => {
  const updates = [
    event(2, 'acp', {
      projection_key: 'acp_status:thread',
      acp: { id: 'thread', state: 'running', goal_requested: true },
    }),
    event(3, 'goal_update', {
      projection_key: 'goal_update:thread',
      goal: { objective: 'Finish the work', status: 'active' },
      acp: undefined,
    }),
    event(4, 'acp_tool', {
      projection_key: 'acp_tool:thread:read',
      acp: { id: 'thread', state: 'running', goal_requested: true, tool_calls: [{ id: 'read' }] },
    }),
    event(5, 'acp', {
      projection_key: 'acp_status:thread',
      acp: { id: 'thread', state: 'cancelled', goal_requested: false },
    }),
    event(6, 'goal_clear', { projection_key: 'goal_update:thread', acp: undefined }),
  ]
  const active = updates.slice(0, 3).reduce(mergeSessionEvent, [])
  expect(deriveSessionView(data(), active).goalActive).toBe(true)
  expect(deriveSessionView(data(), active).goalRequested).toBe(true)

  const cleared = updates.reduce(mergeSessionEvent, [])
  expect(cleared.map((item) => item.seq)).toEqual([5, 6, 4])
  const view = deriveSessionView(data(), cleared)
  expect(view.goalActive).toBe(false)
  expect(view.goalRequested).toBe(false)

  const normalTurn = mergeSessionEvent(cleared, event(7, 'acp', {
    projection_key: 'acp_status:thread',
    acp: { id: 'thread', state: 'running', goal_requested: false },
  }))
  expect(deriveSessionView(data(), normalTurn).goalRequested).toBe(false)
  const requestedAgain = mergeSessionEvent(normalTurn, event(8, 'acp', {
    projection_key: 'acp_status:thread',
    acp: { id: 'thread', state: 'running', goal_requested: true },
  }))
  expect(deriveSessionView(data(), requestedAgain).goalRequested).toBe(true)
})

test('refreshed goal clear wins over older streamed events before the stream cache is pruned', () => {
  const oldGoal = event(2, 'goal_update', {
    projection_key: 'goal_update:thread',
    goal: { objective: 'Finish the work', status: 'active' },
    acp: undefined,
  })
  const oldRequest = event(3, 'acp', {
    projection_key: 'acp_status:thread',
    acp: { id: 'thread', state: 'running', goal_requested: true },
  })
  const persisted = [
    event(4, 'acp', {
      projection_key: 'acp_status:thread',
      acp: { id: 'thread', state: 'cancelled', goal_requested: false },
    }),
    event(5, 'goal_clear', { projection_key: 'goal_update:thread', acp: undefined }),
  ]
  for (const live of [[oldGoal, oldRequest], []]) {
    const view = deriveSessionView({ ...data(persisted), acp_goal_requested: false }, live)
    expect(view.goal).toBeUndefined()
    expect(view.goalRequested).toBe(false)
  }
})

test('goal clear disarms a running turn without requiring a cancelled status', () => {
  const running = event(2, 'acp', {
    acp: { id: 'thread', state: 'running', goal_requested: true },
  })
  const view = deriveSessionView({ ...data([running]), acp_goal_requested: true }, [
    event(3, 'goal_clear', { acp: undefined }),
  ])
  expect(view.goalRequested).toBe(false)
  expect(view.goalActive).toBe(false)
  expect(view.transcriptEvents.find((item) => item.seq === 2).acp.state).toBe('running')
})
