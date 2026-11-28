import { describe, expect, test } from 'bun:test'
import { botChat, connectedTaskThreads, dragSections, lastBotId, placePin, rememberBot, taskBrief, taskName, taskState } from './bots'
import { optimisticTranscriptMessages } from './optimisticUserMessage'
import { mergeEarlierHistory } from './sessionHistory'

describe('bot chat log', () => {
  const self = { id: 'gimli', name: 'Gimli' }
  const at = (minute) => `2026-09-30T21:${String(minute).padStart(2, '0')}:00Z`
  const user = (seq, minute, text) => ({ seq, role: 'user', from_user: true, content: text, blocks: [], created_at: at(minute) })
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

  test('authorship governs visibility, including identical human and internal text', () => {
    const text = 'Task status: keep  spaces, a\ttab and\na line break.'
    const human = user(1, 1, text)
    const internal = { ...user(2, 2, text), from_user: false }
    const unknown = { ...user(3, 3, text), from_user: undefined }
    const events = [said(4, 'A deliberate public delivery.'), woke(5, 'message_received', 'Worker'), event(6, { type: 'room_message', room_message: { speaker: 'user', text } })]
    expect(shape(botChat([human, internal, unknown], events, self, []).entries)).toEqual([
      `user: ${text}`, 'bot: A deliberate public delivery.', `user: ${text}`,
    ])
    expect(botChat([human, internal], [], self, []).work.doing).toBeUndefined()
  })

  test('private narration stays private when the bot sent no answer', () => {
    const messages = [user(1, 1, 'hi')]
    const events = [wrote(2, 'Checking.'), woke(3, 'message_sent', 'Pip'), tool(4), wrote(5, 'Hi! What should we work on?')]
    expect(shape(botChat(messages, events, self, []).entries)).toEqual(['user: hi'])
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
    expect(entries.map((entry) => entry.kind)).toEqual(['user', 'bot', 'app', 'user', 'app'])
    expect(entries[2].app).toBe(tasks)
    expect(entries.at(-1).at).toBe(at(8))
  })

  test('a change to its files does not end the routine turn it happens in', () => {
    const app = { server_id: 'crm', tool: 'show_crm', arguments: { path: '/o/deals' }, result: { content: [] } }
    const events = [woke(2, 'routine', 'Digest'), woke(3, 'file_changed', 'MEMORY.md'), event(4, { type: 'mcp_app', mcp_app: app })]
    expect(botChat([], events, self, opens).entries.map((entry) => entry.kind)).toEqual(['app'])
  })

  test('artifacts retain their source event and public deliveries survive pages without turn context', () => {
    const legacy = event(2, { type: 'artifact', artifact: { title: 'Original map', widget_code: '<svg></svg>', presented: true } })
    const privateMap = event(4, { type: 'artifact', artifact: { title: 'Private map', widget_code: '<svg></svg>' } })
    const delivered = event(6, { type: 'artifact', artifact: { title: 'Routine map', widget_code: '<svg></svg>', presented: true } })
    const entries = botChat([user(1, 1, 'Draw the chain')], [legacy, woke(3, 'message_received', 'Peer'), privateMap, woke(5, 'routine', 'Digest'), delivered], self, []).entries
    expect(entries.map((entry) => entry.kind)).toEqual(['user', 'artifact', 'artifact'])
    expect(entries[1].event).toBe(legacy)
    expect(entries[1].key).toBe(`${legacy.session_id}:${legacy.seq}`)
    expect(entries[2].event).toBe(delivered)
    expect(botChat([], [delivered], self, []).entries[0].event).toBe(delivered)
    expect(botChat([], [privateMap], self, []).entries).toEqual([])
    expect(botChat([user(1, 1, 'An earlier public turn')], [privateMap], self, []).entries.map((entry) => entry.kind)).toEqual(['user'])
    expect(botChat([], [woke(3, 'routine', 'A loaded routine'), privateMap], self, []).entries).toEqual([])
  })

  test('merging clipped private and routine boundaries cannot change resolved artifact visibility', () => {
    for (const kind of ['message_received', 'group', 'routine']) {
      const boundary = woke(2, kind, 'Source turn')
      const artifact = event(4, { type: 'artifact', artifact: { title: 'Historical map', widget_code: '<svg></svg>', presented: kind === 'routine' } })
      const latest = { session: { id: self.id, source_type: 'bot' }, messages: kind === 'routine' ? [] : [user(1, 1, 'An unrelated earlier turn')], events: [artifact] }
      const merged = mergeEarlierHistory(latest, { ...latest, messages: [], events: [boundary] })
      const visible = (page) => botChat(page.messages, page.events, self, []).entries.filter((entry) => entry.kind === 'artifact').map((entry) => entry.key)
      expect(visible(latest)).toEqual(kind === 'routine' ? [`${artifact.session_id}:${artifact.seq}`] : [])
      expect(visible(merged)).toEqual(visible(latest))
    }
  })

  test('group turns and routine runs stay out of the chat unless the bot speaks', () => {
    const { entries } = botChat(
      [user(1, 1, 'hi')],
      [said(2, 'Hey.'), woke(3, 'group', 'Team'), wrote(4, 'PASS'), woke(5, 'routine', 'Digest'), wrote(6, 'Nothing new.'), woke(7, 'message_sent', 'dr eggbot'), event(8, { type: 'agent_switch', content: 'codex' })],
      self,
      [],
    )
    expect(shape(entries)).toEqual(['user: hi', 'bot: Hey.', '· agent_switch'])
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
  const earlier = { seq: 7, role: 'user', from_user: true, content: 'check the inbox', blocks: [], created_at: '2026-10-18T19:00:00Z' }
  const typed = { baselineMessageSeq: 7, message: { role: 'user', from_user: true, content: 'and the calendar', blocks: [], created_at: '2026-10-18T19:01:00.100Z' } }
  const saved = { seq: 8, role: 'user', from_user: true, content: 'and the calendar', blocks: [], created_at: '2026-10-18T19:01:00.450Z' }
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
  const messages = [{ seq: 1, role: 'user', from_user: true, content: 'post it', blocks: [], created_at: at(1) }]
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

test('a finished worker does not make an open task look complete', () => {
  expect(taskState({ status: 'in_progress', threads: [{ state: 'finished' }] })).toEqual(['Not delivered yet', 'In progress', true])
})

test('task state distinguishes active work and blockers from worker completion', () => {
  expect(taskState({ status: 'in_progress', moving: true, threads: [{ state: 'working' }] })).toEqual(['Working', 'In progress'])
  expect(taskState({ status: 'pending', blocked: true, moving: true })).toEqual(['Waiting on another task', 'Queued', true])
  expect(taskState({ status: 'waiting_for_user', moving: true })).toEqual(['Waiting for you', 'Waiting for you'])
  expect(taskState({ status: 'stopped' })).toEqual(['Stopped', 'Stopped'])
  expect(taskState({ status: 'pending' })).toEqual(['Not started', 'Queued'])
  expect(taskState({ status: 'in_progress', stalled: true })).toEqual(['Needs follow-up', 'In progress', true])
})

test('a closed task says how it closed, whatever its threads did', () => {
  expect(taskState({ status: 'completed', threads: [{ state: 'finished' }] })).toEqual(['Done', 'Done'])
  expect(taskState({ status: 'cancelled', threads: [{ state: 'failed' }] })).toEqual(['Dropped', 'Stopped', true])
})

test('taskName uses the concise name and falls back to a clean single line for older unnamed tasks', () => {
  expect(taskName({ name: '  Connected threads  ', request: 'Full long request\nwith multiple lines' })).toBe('Connected threads')
  expect(taskName({ request: 'Overnight stress run; done when all checks pass' })).toBe('Overnight stress run')
  expect(taskName({ request: '\n  Fix   bot task list\n\nSecond paragraph details' })).toBe('Fix bot task list')
})

test('taskBrief keeps short text whole and cuts long text at a sentence, else at a word', () => {
  expect(taskBrief('  Waiting on\n the staging  deploy ')).toBe('Waiting on the staging deploy')
  const sentences = 'The diff review is checking that results survive a restart. The design review already passed. After both land, the bot rechecks the sidebar counts and the follow-up conversation links, then reports back.'
  expect(taskBrief(sentences)).toBe('The diff review is checking that results survive a restart. The design review already passed.')
  expect(taskBrief('Continue the existing thread to redesign the task modal so opening a task shows a clean view of its title and state', 60)).toBe('Continue the existing thread to redesign the task modal so…')
})

test('connectedTaskThreads lists direct links and nested descendants in display order without duplicates', () => {
  const task = {
    threads: [
      { id: 'worker-1', title: 'Worker 1', state: 'finished' },
      { id: 'child-1', parent_id: 'worker-1', title: 'Child 1', state: 'working' },
      { id: 'worker-2', title: 'Worker 2', state: 'failed' },
      { id: 'idle-child', parent_id: 'worker-2', title: 'Idle child', state: 'idle' },
      { id: 'worker-1', title: 'Duplicate worker', state: 'finished' },
    ],
  }
  const started = {
    'worker-1': [{ kind: 'side_chat', at: '2026-11-22T09:01:00Z', title: 'Child 1', status: 'running', running: true, thread_id: 'child-1' }],
    'child-1': [{ kind: 'side_chat', at: '2026-11-22T09:02:00Z', title: 'Grandchild 1', status: 'idle', thread_id: 'grandchild-1' }],
  }
  const connected = connectedTaskThreads(task, started)
  expect(connected.map((row) => [row.id, row.depth])).toEqual([
    ['worker-1', 0],
    ['child-1', 1],
    ['grandchild-1', 2],
    ['worker-2', 0],
    ['idle-child', 1],
  ])
  expect(connected[1].thread?.state).toBe('working')
  expect(connected[2].activity?.title).toBe('Grandchild 1')
  expect(connected[4].thread?.state).toBe('idle')
})

test('connectedTaskThreads handles parent_id trees without started and stays cycle-safe', () => {
  const fromParentIds = connectedTaskThreads({
    threads: [
      { id: 'root', title: 'Root', state: 'finished' },
      { id: 'child', parent_id: 'root', title: 'Child', state: 'gone' },
      { id: 'grandchild', parent_id: 'child', title: 'Grandchild', state: 'background' },
    ],
  })
  expect(fromParentIds.map((row) => [row.id, row.depth, row.thread?.state])).toEqual([
    ['root', 0, 'finished'],
    ['child', 1, 'gone'],
    ['grandchild', 2, 'background'],
  ])

  const cyclic = connectedTaskThreads(
    {
      threads: [
        { id: 'a', title: 'Thread A', state: 'working' },
        { id: 'b', title: 'Thread B', state: 'finished' },
      ],
    },
    {
      a: [{ kind: 'side_chat', at: '2026-11-22T09:01:00Z', title: 'Thread B', thread_id: 'b' }],
      b: [{ kind: 'side_chat', at: '2026-11-22T09:02:00Z', title: 'Thread A', thread_id: 'a' }],
    },
  )
  expect(cyclic.map((row) => [row.id, row.depth])).toEqual([
    ['a', 0],
    ['b', 1],
  ])

  const viaStartedIntermediate = connectedTaskThreads(
    {
      threads: [
        { id: 'w1', title: 'Worker 1', state: 'finished' },
        { id: 'g1', parent_id: 'c1', title: 'Grandchild 1', state: 'working' },
      ],
    },
    {
      w1: [{ kind: 'side_chat', at: '2026-11-22T09:01:00Z', title: 'Child 1', status: 'idle', thread_id: 'c1' }],
    },
  )
  expect(viaStartedIntermediate.map((row) => [row.id, row.depth])).toEqual([
    ['w1', 0],
    ['c1', 1],
    ['g1', 2],
  ])
})

test('remembered bot selection stays isolated per workspace', () => {
  const prev = Object.getOwnPropertyDescriptor(globalThis, 'localStorage')
  const stored = new Map()
  Object.defineProperty(globalThis, 'localStorage', {
    configurable: true,
    writable: true,
    value: {
      getItem: (key) => stored.get(key) ?? null,
      setItem: (key, value) => stored.set(key, value),
      removeItem: (key) => stored.delete(key),
    },
  })
  try {
    rememberBot('bot_local', 'local')
    rememberBot('bot_cas', 'ws_cas')
    rememberBot('bot_personal', 'ws_personal')
    expect(lastBotId('local')).toBe('bot_local')
    expect(lastBotId()).toBe('bot_local')
    expect(lastBotId('ws_cas')).toBe('bot_cas')
    expect(lastBotId('ws_personal')).toBe('bot_personal')
    expect(lastBotId('ws_empty')).toBeUndefined()
  } finally {
    if (prev) Object.defineProperty(globalThis, 'localStorage', prev)
    else delete globalThis.localStorage
  }
})
