import { expect, test } from 'bun:test'
import { deriveSessionView } from '@/components/session/sessionView'

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
    expect(deriveSessionView(data(current), []).acpThinking).toBe(expected[count])
    expect(deriveSessionView(data(current.slice(0, -1)), current.slice(-1)).acpThinking).toBe(expected[count])
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
  expect(deriveSessionView(data([first, tool]), [next, ...unrelated]).acpThinking).toBe(true)
})

test('a new user turn or an aggregate snapshot cannot inherit an old thinking signal', () => {
  const old = event(2, 'acp_thought', { acp: { id: 'thread', thought: 'Old reasoning' } })
  expect(deriveSessionView(data([old], 3), []).acpThinking).toBe(false)
  expect(deriveSessionView({ ...data(), acp_thought: 'Unknown order' }, []).acpThinking).toBe(false)
})
