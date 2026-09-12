import { describe, expect, test } from 'bun:test'
import { voiceChatContext } from './transcript'
import { taskFinished, voiceTaskUpdate } from './delegation'

const user = (seq, content, requestId = 'delegation') => ({ seq, role: 'user', content, blocks: [{ type: 'voice_context', id: 'call', request_id: requestId }], created_at: new Date(seq * 1000).toISOString() })
const answer = (seq, content) => ({ seq, role: 'assistant', content, created_at: new Date(seq * 1000).toISOString() })
const task = () => ({ id: 'delegation', callId: 'call' })
const snapshot = (messages, extra = {}) => ({ messages, events: [], ...extra, session: { id: 'thread', status: 'running', ...extra.session } })

const event = (seq, content, run, state = 'running') => ({
  seq, session_id: 'thread', type: 'acp_message', content,
  at: new Date(seq * 1000).toISOString(),
  projection_key: `acp_text:thread:thread:acp_message:message:${run}`,
  projection_op: 'append',
  acp: { id: 'thread', state, text_run_id: `message:${run}` },
})

describe('voice follows the selected thread without treating speech as task completion', () => {
  test('ignores answers from before this request and reports the durable final answer', () => {
    const request = task()
    expect(voiceTaskUpdate(request, snapshot([answer(9, 'Old result')])).state).toBe('running')
    expect(voiceTaskUpdate(request, snapshot([user(11, 'Check the repository'), answer(12, 'Working…')])).state).toBe('running')
    expect(voiceTaskUpdate(request, snapshot([user(11, 'Check the repository'), answer(13, 'Verified result')], { session: { status: 'idle' } }))).toEqual({ state: 'completed', text: 'Verified result' })
  })

  test('a newer user instruction supersedes an unfinished answer', () => {
    const request = task()
    const update = voiceTaskUpdate(request, snapshot([user(11, 'Check the repository'), answer(12, 'I will check'), user(13, 'Check a different branch')]))
    expect(update.state).toBe('superseded')
    expect(update.text).not.toContain('I will check')
  })

  test('approval stays pending and never becomes an automatic approval', () => {
    const update = voiceTaskUpdate(task(), snapshot([user(11, 'Check the repository')], { acp_permissions: [{ status: 'pending', title: 'Allow command?' }] }))
    expect(update.state).toBe('approval')
    expect(taskFinished(update)).toBe(false)
  })

  test('a cancelled or failed task never speaks its partial output as success', () => {
    const messages = [user(11, 'Check the repository'), answer(12, 'Partial output')]
    expect(voiceTaskUpdate(task(), snapshot(messages, { session: { status: 'interrupted' }, acp_state: 'cancelled' })).state).toBe('cancelled')
    expect(voiceTaskUpdate(task(), snapshot(messages, { session: { status: 'error', error: 'Provider disconnected' } })).state).toBe('failed')
  })

  test('typed queue entries do not affect voice task tracking', () => {
    const update = voiceTaskUpdate(task(), snapshot([], { session: { queued_messages: [{ id: 'typed', text: 'Later task' }] } }))
    expect(update.state).toBe('running')
    expect(taskFinished(update)).toBe(false)
  })

  test('an accepted request stays running before its user row appears', () => {
    const update = voiceTaskUpdate(task(), snapshot([], { acp_state: 'idle' }))
    expect(update.state).toBe('running')
    expect(taskFinished(update)).toBe(false)
  })

  test('Codex results are coalesced ACP events, with no assistant message row', () => {
    const request = task()
    const messages = [user(11, 'Check the repository')]
    const events = [
      event(8, 'Old answer', 'old', 'idle'),
      event(12, 'I’ll check the directory again.\n', 'commentary'),
      event(13, 'The check completed successfully. Files: ', 'answer'),
      event(14, '`jaz`, `airpods.html`.', 'answer'),
      { ...event(15, '', 'status', 'idle'), type: 'acp', projection_key: 'acp_status:thread', projection_op: 'replace' },
      { ...event(16, 'Child answer', 'child', 'idle'), acp: { id: 'child', parent_id: 'thread' } },
    ]
    expect(voiceTaskUpdate(request, snapshot(messages, { events: events.slice(0, 4), acp_state: 'running' })).state).toBe('running')
    expect(voiceTaskUpdate(request, snapshot(messages, { events, acp_state: 'running' })).state).toBe('completed')
    expect(voiceTaskUpdate(request, snapshot(messages, { events, session: { status: 'idle' } }))).toEqual({ state: 'completed', text: 'The check completed successfully. Files: `jaz`, `airpods.html`.' })
  })

  test('a completed result still belongs to its request after a newer turn starts', () => {
    const request = task()
    const messages = [user(11, 'Check the repository'), user(15, 'Try again')]
    const events = [event(14, 'First verified result', 'first', 'idle'), event(16, 'New task preamble', 'second')]
    expect(voiceTaskUpdate(request, snapshot(messages, { events }))).toEqual({ state: 'completed', text: 'First verified result' })
  })
})

test('voice context includes the visible ACP conversation and selected workspace, without private or child content', () => {
  const context = voiceChatContext(snapshot([
    { ...user(1, 'Private instructions'), role: 'system' },
    user(2, 'Which files are here?'),
  ], {
    session: { title: 'List files', runtime_ref: { agent: 'codex', cwd: '/workspace/project' }, model: 'selected-model' },
    events: [event(3, 'Files: ', 'answer'), event(4, 'example.txt', 'answer'),
      { ...event(5, 'Private reasoning', 'thought'), type: 'acp_thought' },
      { ...event(6, 'Child content', 'child'), acp: { id: 'child', parent_id: 'thread' } }],
  }))
  expect(context).toContain('Selected agent: codex')
  expect(context).toContain('Selected agent model: selected-model')
  expect(context).toContain('Workspace: /workspace/project')
  expect(context).toContain('user: Which files are here?\nassistant: Files: example.txt')
  expect(context).not.toContain('Private')
  expect(context).not.toContain('Child content')
})

test('bounded voice history preserves the newest answer', () => {
  const messages = Array.from({ length: 20 }, (_, index) => user(index, 'old '.repeat(1000)))
  const context = voiceChatContext(snapshot(messages, { events: [event(25, 'Most recent answer', 'latest')] }))
  expect(context.length).toBeLessThan(16000)
  expect(context).toContain('assistant: Most recent answer')
  expect(context).toContain('Selected agent model: unknown')
})


test('identical spoken requests keep separate request identities', () => {
  const first = user(11, 'Check files', 'first')
  const second = user(14, 'Check files again', 'second')
  const messages = [first, answer(12, 'First answer'), second, answer(15, 'Second answer')]
  const request = { id: 'second', callId: 'call' }
  expect(voiceTaskUpdate(request, snapshot(messages, { session: { status: 'idle' } }))).toEqual({ state: 'completed', text: 'Second answer' })
  expect(request.user.seq).toBe(14)
  const typed = { ...second, blocks: [], content: 'Check files' }
  expect(voiceTaskUpdate({ id: 'second', callId: 'call' }, snapshot([first, typed])).state).toBe('running')
  expect(voiceTaskUpdate({ id: 'first', callId: 'different-call' }, snapshot([first])).state).toBe('running')
})
