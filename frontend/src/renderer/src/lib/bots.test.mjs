import { describe, expect, test } from 'bun:test'
import { botChat, dragSections, owedState, placePin } from './bots'
import { optimisticTranscriptMessages } from './optimisticUserMessage'

describe('owed commitment state', () => {
  const owed = {
    id: 38,
    status: 'in_progress',
    request: 'Implement, review and merge to local main',
    next: 'Review the merged changes',
    opened_at: '2026-10-19T12:00:00Z',
  }

  test('five failed attempts followed by finished work wait on the user without danger and retain their history', () => {
    const threads = [
      ...['implementation', 'review', 'verification', 'integration', 'delivery'].map((title) => ({ id: `${title}-claude`, title, state: 'failed' })),
      ...['implementation', 'review', 'verification', 'integration', 'delivery'].map((title) => ({ id: `${title}-codex`, title, state: 'finished' })),
    ]
    const commitment = { ...owed, status: 'waiting_for_user', threads }
    const history = structuredClone(threads)
    expect(owedState(commitment)).toEqual({ danger: false, standing: 'Waiting on you' })
    expect(commitment.threads).toEqual(history)
  })

  test.each([
    [[], false, ''],
    [['idle', 'gone'], false, ''],
    [['failed'], true, 'A thread failed'],
    [['finished', 'failed'], true, 'A thread failed'],
    [['working', 'failed'], true, 'A thread failed'],
    [['failed', 'finished'], false, 'Result in'],
    [['failed', 'working'], false, 'Working'],
    [['failed', 'background'], false, 'Working'],
    [['failed', 'idle'], true, 'A thread failed'],
    [['failed', 'gone'], true, 'A thread failed'],
    [['failed', 'finished', 'idle', 'gone'], false, 'Result in'],
    [['failed', 'working', 'finished'], false, 'Working'],
  ])('linked threads %j give danger=%s and standing=%s', (states, danger, standing) => {
    const threads = states.map((state, index) => ({ id: `thread-${states.length - index}`, title: 'Work', state }))
    expect(owedState({ ...owed, threads })).toEqual({ danger, standing })
  })

  test.each([
    [{ stalled: true }, true, 'Stalled'],
    [{ moving: true }, false, 'Working'],
    [{ status: 'pending' }, false, 'Not started'],
    [{ status: 'waiting_for_user', for: 'Operations' }, false, 'Waiting on you'],
  ])('commitment standing %j takes precedence after recovery', (fields, danger, standing) => {
    const threads = [{ id: 'first', title: 'Work', state: 'failed' }, { id: 'retry', title: 'Work', state: 'finished' }]
    expect(owedState({ ...owed, ...fields, threads })).toEqual({ danger, standing })
  })

  test('waiting on the user does not clear a current failure', () => {
    const threads = [{ id: 'latest', title: 'Work', state: 'failed' }]
    expect(owedState({ ...owed, status: 'waiting_for_user', threads })).toEqual({ danger: true, standing: 'Waiting on you' })
  })
})

describe('bot chat log', () => {
  const self = { id: 'gimli', name: 'Gimli' }
  const at = (minute) => `2026-09-30T21:${String(minute).padStart(2, '0')}:00Z`
  const user = (seq, minute, text) => ({ seq, role: 'user', content: text, blocks: [], created_at: at(minute) })
  let seq = 0
  const event = (minute, fields) => ({ session_id: 'gimli', seq: ++seq, at: at(minute), ...fields })
  const said = (minute, text) => event(minute, { type: 'room_message', room_message: { speaker: 'bot', bot_id: 'gimli', name: 'Gimli', text } })
  const wrote = (minute, text) => event(minute, { type: 'acp_message', content: text, acp: { id: 'gimli' } })
  const tool = (minute) => event(minute, { type: 'acp', acp: { id: 'gimli', tool_calls: [{ id: 't', title: 'Run rm -rf build' }] } })
  const woke = (minute, kind, label) => event(minute, { type: 'bot_activity', bot_activity: { kind, label } })
  const opens = [{ server_id: 'crm', tool: 'show_crm', type: 'global', title: 'Customers' }]
  const shape = (entries) => entries.map((entry) => entry.kind === 'activity' ? `· ${entry.event.bot_activity?.kind ?? entry.event.type}` : `${entry.kind}: ${entry.text}`)

  test('shows only what was typed and sent, never the work between', () => {
    const { entries } = botChat(
      [user(1, 1, 'move linkin park to in progress')],
      [wrote(2, 'Let me find the issue first.'), tool(3), said(4, 'Moved it.'), wrote(5, 'Done: DEM-6 is In Progress.')],
      self,
      [],
    )
    expect(shape(entries)).toEqual(['user: move linkin park to in progress', 'bot: Moved it.'])
  })

  test('private narration stays private when the bot sent no answer', () => {
    const messages = [user(1, 1, 'hi')]
    const events = [wrote(2, 'Checking.'), woke(3, 'message_sent', 'Pip'), tool(4), wrote(5, 'Hi! What should we work on?')]
    expect(shape(botChat(messages, events, self, []).entries)).toEqual(['user: hi', '· message_sent'])
  })

  test('an app the bot opens for the user shows and stands in for a reply, while its lookups stay private', () => {
    const app = { server_id: 'crm', tool: 'show_crm', arguments: { path: '/o/deals' }, result: { content: [{ type: 'text', text: '{"path":"/o/deals"}' }], structuredContent: { path: '/o/deals' } } }
    const lookup = { server_id: 'crm', tool: 'search_records', arguments: { object: 'deals' }, result: { content: [], structuredContent: { resource_uri: 'ui://jaz-crm/o/deals' } } }
    const events = [wrote(2, 'Looking up the deals.'), event(3, { type: 'mcp_app', mcp_app: lookup }), event(3, { type: 'mcp_app', mcp_app: app }), tool(4), wrote(5, 'Private notes.')]
    const { entries } = botChat([user(1, 1, 'show me deals')], events, self, opens)
    expect(entries.map((entry) => entry.kind)).toEqual(['user', 'app'])
    expect(entries[1].app).toBe(app)
  })

  test('routine presentations reach the user while group and peer work stays private', () => {
    const app = { server_id: 'crm', tool: 'show_crm', arguments: { path: '/o/deals' }, result: { content: [] } }
    const tasks = { server_id: 'tasks', tool: 'show_issues', presented: true, arguments: {}, result: { content: [{ type: 'resource_link', uri: 'ui://tasks/issue', name: 'Tasks', mimeType: 'text/html;profile=mcp-app' }] } }
    const events = [said(2, 'Hey.'), woke(3, 'group', 'Team'), event(4, { type: 'mcp_app', mcp_app: app }), woke(4, 'message_received', 'Pip'), event(4, { type: 'mcp_app', mcp_app: app }), woke(5, 'routine', 'Digest'), event(6, { type: 'mcp_app', mcp_app: tasks }), wrote(6, 'Private completion.'), event(8, { type: 'mcp_app', mcp_app: tasks }), wrote(9, 'Repeated task list.')]
    const { entries } = botChat([user(1, 1, 'hi'), user(2, 7, 'show me deals')], events, self, opens)
    expect(entries.map((entry) => entry.kind)).toEqual(['user', 'bot', 'activity', 'app', 'user', 'app'])
    expect(entries[3].app).toBe(tasks)
    expect(entries.at(-1).at).toBe(at(8))
  })

  test('a change to its files does not end the routine turn it happens in', () => {
    const app = { server_id: 'crm', tool: 'show_crm', arguments: { path: '/o/deals' }, result: { content: [] } }
    const events = [woke(2, 'routine', 'Digest'), woke(3, 'file_changed', 'MEMORY.md'), event(4, { type: 'mcp_app', mcp_app: app })]
    expect(botChat([], events, self, opens).entries.map((entry) => entry.kind)).toEqual(['app'])
  })

  test('group turns and routine runs stay out of the chat unless the bot speaks', () => {
    const { entries } = botChat(
      [user(1, 1, 'hi')],
      [said(2, 'Hey.'), woke(3, 'group', 'Team'), wrote(4, 'PASS'), woke(5, 'routine', 'Digest'), wrote(6, 'Nothing new.'), woke(7, 'message_sent', 'dr eggbot'), event(8, { type: 'agent_switch', content: 'codex' })],
      self,
      [],
    )
    expect(shape(entries)).toEqual(['user: hi', 'bot: Hey.', '· message_sent', '· agent_switch'])
  })

  test('a group shows when a member could not be reached', () => {
    const { entries } = botChat([], [woke(2, 'unreachable', 'CEO · unknown model provider')], self, [])
    expect(shape(entries)).toEqual(['· unreachable'])
  })

  test('work labels follow the active conversation without showing private notes', () => {
    const events = [said(2, 'Hey.'), woke(3, 'message_received', 'Pip'), wrote(4, 'Reading the CRM.\n\nChecking two more threads.')]
    expect(botChat([user(1, 1, 'hi')], events, self, []).work).toEqual({ doing: "working on Pip's message" })
    expect(botChat([user(1, 1, 'hi')], [...events, woke(5, 'routine', 'Say hi')], self, []).work).toEqual({ doing: 'running Say hi' })
    expect(botChat([user(1, 1, 'hi'), user(2, 6, 'still there?')], events, self, []).work.doing).toBeUndefined()
  })
})

// A message the person sends shows at once, is saved with a number of its
// own and is then reloaded with the server's time; it stays one message
// throughout, so the chat neither remounts it nor plays its arrival again.
test('a sent message keeps its place in the chat from send to save', () => {
  const self = { id: 'gimli', name: 'Gimli' }
  const earlier = { seq: 7, role: 'user', content: 'check the inbox', blocks: [], created_at: '2026-10-18T19:00:00Z' }
  const typed = { baselineMessageSeq: 7, message: { role: 'user', content: 'and the calendar', blocks: [], created_at: '2026-10-18T19:01:00.100Z' } }
  const saved = { seq: 8, role: 'user', content: 'and the calendar', blocks: [], created_at: '2026-10-18T19:01:00.450Z' }
  const keys = (messages) => botChat(messages, [], self, []).entries.map((entry) => entry.key)
  const sending = keys(optimisticTranscriptMessages([earlier], typed))
  const accepted = keys(optimisticTranscriptMessages([earlier, saved], typed))
  const reloaded = keys([earlier, saved])
  expect(accepted).toEqual(sending)
  expect(reloaded).toEqual(sending)
})

test('a question the bot asks shows in its chat, carries its answer and stands in for a written reply', () => {
  const at = (minute) => `2026-09-30T21:${String(minute).padStart(2, '0')}:00Z`
  const asked = (minute, type, extra = {}) => ({ session_id: 'gimli', seq: minute, at: at(minute), type, permission: { id: 'q1', title: 'Which account?', questions: [{ id: 'a', question: 'Which account?' }], ...extra } })
  const messages = [{ seq: 1, role: 'user', content: 'post it', blocks: [], created_at: at(1) }]
  const events = [
    { session_id: 'gimli', seq: 2, at: at(2), type: 'acp_message', content: 'Checking accounts.', acp: { id: 'gimli' } },
    asked(3, 'permission_request'),
    asked(4, 'permission_request'),
  ]
  expect(botChat(messages, events, { id: 'gimli', name: 'Gimli' }, []).waiting).toBe(true)
  const { entries, waiting } = botChat(messages, [...events, asked(5, 'permission_response', { status: 'resolved' })], { id: 'gimli', name: 'Gimli' }, [])
  expect(entries.map((entry) => entry.kind)).toEqual(['user', 'question'])
  expect(entries[1].event.seq).toBe(4)
  expect(entries[1].answer?.status).toBe('resolved')
  expect(waiting).toBe(false)
})

test('a dragged pin lands beside the tile under it, and holds still over itself or empty space', () => {
  expect(placePin(['a', 'b', 'c'], 'c', 'a', false)).toEqual(['c', 'a', 'b'])
  expect(placePin(['a', 'b', 'c'], 'a', 'b', true)).toEqual(['b', 'a', 'c'])
  expect(placePin(['a', 'b'], 'x', 'a', true)).toEqual(['a', 'x', 'b'])
  expect(placePin(['a', 'b'], 'x', undefined, false)).toEqual(['a', 'b', 'x'])
  expect(placePin(['a', 'b', 'c'], 'b', 'b', true)).toEqual(['a', 'b', 'c'])
  expect(placePin(['a', 'b'], 'a', undefined, false)).toEqual(['a', 'b'])
})

test('a drag previews where the bot lands and keeps it, hidden, where the drag began', () => {
  const bot = (id, pinned) => ({ id, name: id, pinned, updated_at: '2026-10-01T09:00:00Z' })
  const bots = [bot('a', 1), bot('b', 2), bot('c')]
  const shape = ({ tiles, rows }) => [tiles, rows].map((section) => section.map(({ bot, hidden }) => bot.id + (hidden ? ' hidden' : '')))
  expect(shape(dragSections(bots, ['a', 'b']))).toEqual([['a', 'b'], ['c']])
  expect(shape(dragSections(bots, ['a', 'b'], { id: 'b', pins: ['b', 'a'] }))).toEqual([['b', 'a'], ['c']])
  expect(shape(dragSections(bots, ['a', 'b'], { id: 'a', pins: ['b'] }))).toEqual([['b', 'a hidden'], ['a', 'c']])
  expect(shape(dragSections(bots, ['a', 'b'], { id: 'c', pins: ['a', 'c', 'b'] }))).toEqual([['a', 'c', 'b'], ['c hidden']])
})
