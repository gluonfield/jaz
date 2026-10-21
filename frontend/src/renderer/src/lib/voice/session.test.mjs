import { afterEach, expect, test } from 'bun:test'
import { build } from 'bun'
import duplicateTurns from './testdata/duplicate-turn.json'
import { setTimeout as delay } from 'node:timers/promises'

const bundle = await build({ entrypoints: [new URL('./session.ts', import.meta.url).pathname], target: 'browser', format: 'cjs', define: { 'import.meta.env': '{}' } })
if (!bundle.success) {
  throw new AggregateError(bundle.logs)
}
const compiled = new Function('module', 'exports', await bundle.outputs[0].text())
const originals = new Map()
let session

function replace(name, value) {
  if (!originals.has(name)) {
    originals.set(name, Object.getOwnPropertyDescriptor(globalThis, name))
  }
  Object.defineProperty(globalThis, name, { configurable: true, writable: true, value })
}

afterEach(async () => {
  session?.dispose()
  await delay(5)
  for (const [name, descriptor] of originals) {
    if (descriptor) {
      Object.defineProperty(globalThis, name, descriptor)
    } else {
      delete globalThis[name]
    }
  }
  originals.clear()
})

async function fixture({ autoAck = true, history, lostResponse = false, rejectedInput = false } = {}) {
  const requests = []
  const wire = []
  const states = []
  const snapshot = history ?? { session: { id: 'thread', title: 'Prime', status: 'running', source_type: 'bot' }, messages: [], events: [], acp_state: 'running', acp_permissions: [], latest_event_seq: 0 }
  const subscriptions = []
  let seq = snapshot.latest_event_seq
  let userTurnId
  let turn = 0
  const track = { enabled: true, stop() {} }
  const media = { getTracks: () => [track], getAudioTracks: () => [track] }
  const channel = {
    readyState: 'open',
    send(data) {
      const event = JSON.parse(data)
      wire.push(event)
      if (autoAck && event.type.endsWith('context.append')) {
        queueMicrotask(() => channel.onmessage({ data: JSON.stringify({ type: event.type + 'ed', delegation_item_id: event.delegation_item_id }) }))
      }
    },
    close() {},
  }
  replace('window', { location: { origin: 'http://voice.invalid' } })
  replace('localStorage', { getItem: () => null })
  replace('navigator', { platform: 'MacIntel', mediaDevices: { getUserMedia: async () => media } })
  replace('Audio', class extends EventTarget {
    play() {
      throw new Error('This test must never play audio')
    }
    pause() {}
  })
  replace('AudioContext', class {
    createAnalyser() {
      return {}
    }
    createMediaStreamSource() {
      return { connect() {} }
    }
    async resume() {}
    async close() {}
  })
  replace('RTCPeerConnection', class {
    iceGatheringState = 'complete'
    createDataChannel() {
      return channel
    }
    addTrack() {}
    async createOffer() {
      return { type: 'offer', sdp: 'v=0\r\noffer' }
    }
    async setLocalDescription(offer) {
      this.localDescription = offer
    }
    async setRemoteDescription() {
      queueMicrotask(() => channel.onmessage({ data: JSON.stringify({ type: 'session.started' }) }))
    }
    close() {}
  })
  replace('EventSource', class {
    constructor() {
      subscriptions.push(this)
    }
    close() {}
  })
  replace('fetch', async (url, init = {}) => {
    const path = new URL(url).pathname
    const body = init.body && JSON.parse(init.body)
    requests.push({ path, body })
    let value = {}
    if (path.endsWith('/messages')) {
      value = structuredClone(snapshot)
    } else if (path.endsWith('/connect')) {
      value = { provider: 'openai', sdp: 'v=0\r\nanswer' }
    } else if (path.endsWith('/agent/input')) {
      if (rejectedInput) {
        rejectedInput = false
        return Response.json({ error: 'agent cannot accept this input' }, { status: 409 })
      }
      const context = body.contexts[0]
      snapshot.messages.push({ seq: snapshot.messages.length + 1, role: 'user', content: body.message, created_at: new Date().toISOString(), blocks: [{ type: 'voice_context', id: context.id, request_id: context.request_id }] })
      if (lostResponse) {
        lostResponse = false
        throw new TypeError('HTTP response lost after admission')
      }
    } else if (path.endsWith('/voice/transcript') || path.endsWith('/voice/diagnostics')) {
      return new Response(null, { status: 204 })
    } else {
      throw new Error(`Unexpected request: ${path}`)
    }
    return Response.json(value)
  })
  const module = { exports: {} }
  compiled(module, module.exports)
  const { VoiceSession } = module.exports
  session = new VoiceSession('thread', (state) => states.push(state), () => {})
  session.start()
  await delay(10)
  expect(states.at(-1).phase).toBe('listening')
  const emit = (event) => channel.onmessage({ data: JSON.stringify(event) })
  const speak = (text, id = `turn-${++turn}`) => {
    userTurnId = id
    emit({ type: 'turn.created', turn: { id, role: 'user' } })
    emit({ type: 'input_transcript.added', item: { text, user_bidi_turn_id: id } })
    emit({ type: 'turn.done', turn: { id, role: 'user', transcript: text } })
  }
  const agent = (event) => {
    const record = { session_id: 'thread', at: new Date().toISOString(), seq: ++seq, ...event }
    snapshot.events.push(record)
    snapshot.latest_event_seq = record.seq
    subscriptions.at(-1).onmessage({ data: JSON.stringify(record) })
  }
  const texts = () => wire.map((event) => event.content?.[0]?.text ?? '').join('')
  const speakable = () => wire.filter((event) => event.channel === 'speakable').map((event) => event.content[0].text).join('')
  const delegate = (text, id = crypto.randomUUID(), turnId = userTurnId) => emit({ type: 'delegation.created', item: { id, user_bidi_turn_id: turnId, target: 'client', content: [{ type: 'input_text', text }] } })
  return { requests, wire, states, snapshot, channel, emit, speak, agent, texts, speakable, delegate }
}

const inputs = (f) => f.requests.filter((request) => request.path.endsWith('/agent/input'))

function ask(f, id = 'question', text = 'Do you mean Hadrian?') {
  const permission = { id, session_id: 'thread', after_message_seq: f.snapshot.messages.at(-1)?.seq ?? 0, turn_started_at: '2026-10-21T10:00:00Z', status: 'pending', questions: [{ id: 'manufacturer', question: text }] }
  f.snapshot.acp_permissions = [permission]
  f.agent({ type: 'permission_request', permission })
  return permission
}

function teardown(f, permission, origin = 'turn_cleanup') {
  f.snapshot.acp_permissions = []
  f.snapshot.session.status = 'idle'
  f.snapshot.acp_state = 'idle'
  f.agent({ type: 'acp', acp: { id: 'thread', state: 'idle', stop_reason: 'end_turn' } })
  f.agent({ type: 'permission_response', permission: { ...permission, status: 'cancelled', origin } })
}

test('teardown precedes speech: raw reply is admitted without a waiter or provider delegation', async () => {
  const f = await fixture()
  teardown(f, ask(f))
  f.agent({ type: 'room_message', room_message: { speaker: 'bot', bot_id: 'thread', text: 'I have the earlier task ready to continue.' } })
  f.speak('Yeah', 'late-reply')
  await delay(20)
  expect(inputs(f)).toHaveLength(1)
  expect(inputs(f)[0].body.message).toBe('Yeah')
  const context = inputs(f)[0].body.contexts[0]
  expect(context.request_id).toBe('late-reply')
  expect(context.text).toContain('Do you mean Hadrian?')
  expect(context.text).toContain('turn_cleanup')
  expect(context.text).toContain('2026-10-21T10:00:00Z')
  expect(context.text).toContain('User: Yeah')
  expect(context.text).toContain('I have the earlier task ready to continue.')
  expect(f.requests.some((request) => request.path.endsWith('/interactive-response'))).toBe(false)
  f.agent({ type: 'room_message', room_message: { speaker: 'bot', bot_id: 'thread', text: 'I searched memory and asked for clarification.' } })
  expect(f.wire.filter((event) => event.channel === 'speakable').every((event) => event.type === 'session.context.append' && !event.delegation_item_id)).toBe(true)
  f.delegate('Yeah', 'actual-delegation', 'late-reply')
  await delay(10)
  expect(inputs(f)).toHaveLength(1)
  f.agent({ type: 'room_message', room_message: { speaker: 'bot', bot_id: 'thread', text: 'Here is the later result.' } })
  expect(f.wire.some((event) => event.channel === 'speakable' && event.delegation_item_id === 'actual-delegation')).toBe(true)
})

test('a finalized native transcript without a start event retains its turn identity', async () => {
  const f = await fixture()
  teardown(f, ask(f))
  f.emit({ type: 'turn.done', turn: { id: 'final-only', role: 'user', transcript: 'Yeah' } })
  f.delegate('Yeah', 'real-item-1', 'final-only')
  f.delegate('Yeah', 'real-item-2', 'final-only')
  await delay(10)
  expect(inputs(f)).toHaveLength(1)
  expect(inputs(f)[0].body.contexts[0].request_id).toBe('final-only')
})

test.each(['Yeah', 'What is the time in London?', 'stop', 'Skip that question', 'I meant another company'])('pending and retained questions admit ordinary raw words: %s', async (text) => {
  for (const retained of [false, true]) {
    const f = await fixture()
    const permission = ask(f)
    if (retained) {
      teardown(f, permission)
    }
    f.speak(text)
    await delay(10)
    expect(inputs(f)).toHaveLength(1)
    expect(inputs(f)[0].body.message).toBe(text)
    expect(inputs(f)[0].body.contexts[0].text).toContain('Do you mean Hadrian?')
    expect(f.requests.some((request) => request.path.endsWith('/interactive-response'))).toBe(false)
    session.dispose()
  }
})

test('remount reconstructs cleanup or unknown origin; explicit Stop retires the question', async () => {
  for (const origin of ['turn_cleanup', 'unknown', 'user_stop']) {
    const f = await fixture()
    teardown(f, ask(f), origin === 'user_stop' ? 'unknown' : origin)
    if (origin === 'user_stop') {
      f.agent({ type: 'acp', acp: { id: 'thread', state: 'running', origin: 'user_stop' } })
      f.agent({ type: 'acp', acp: { id: 'thread', state: 'idle' } })
    }
    session.dispose()
    const next = await fixture({ history: structuredClone(f.snapshot) })
    next.speak('Yeah')
    await delay(10)
    expect(inputs(next)).toHaveLength(origin === 'user_stop' ? 0 : 1)
    if (origin !== 'user_stop') {
      expect(inputs(next)[0].body.contexts[0].text).toContain(origin)
    }
    session.dispose()
  }
})

test('capture freezes the original question before a newer question arrives', async () => {
  const f = await fixture()
  ask(f)
  f.emit({ type: 'turn.created', turn: { id: 'old-reply', role: 'user' } })
  ask(f, 'new-question', 'Delete the project?')
  f.emit({ type: 'turn.done', turn: { id: 'old-reply', role: 'user', transcript: 'Yeah' } })
  await delay(10)
  expect(inputs(f)[0].body.contexts[0].text).toContain('Do you mean Hadrian?')
  expect(inputs(f)[0].body.contexts[0].text).not.toContain('Delete the project?')
})

test('explicit answer, skip or a newer ordinary input consumes automatic eligibility', async () => {
  for (const resolution of ['answer', 'skip', 'new-input']) {
    const f = await fixture()
    const permission = ask(f)
    if (resolution === 'new-input') {
      f.snapshot.messages.push({ seq: 1, role: 'user', content: 'another task', created_at: f.snapshot.events.at(-1).at })
      f.agent({ type: 'session' })
      await delay(10)
    } else {
      f.snapshot.acp_permissions = []
      f.agent({ type: 'permission_response', permission: { ...permission, status: 'selected', selected_option_id: 'answered', answers: resolution === 'skip' ? {} : { manufacturer: ['Yes'] } } })
    }
    f.speak('Yeah')
    await delay(10)
    expect(inputs(f)).toHaveLength(0)
    session.dispose()
  }
})

test.each([true, false])('one admission in either arrival order; delegation first: %s', async (first) => {
  const f = await fixture()
  teardown(f, ask(f))
  if (first) {
    f.delegate('A provider interpretation', 'item-1', 'same-turn')
  }
  f.speak('Yeah', 'same-turn')
  if (!first) {
    f.delegate('A provider interpretation', 'item-1', 'same-turn')
  }
  f.delegate('Yeah', 'item-2', 'same-turn')
  await delay(10)
  expect(inputs(f)).toHaveLength(1)
  expect(inputs(f)[0].body.contexts[0].request_id).toBe('same-turn')
  expect(inputs(f)[0].body.message).toBe('Yeah')
  f.speak('What changed?', 'new-turn')
  f.delegate('What changed?', 'item-3', 'new-turn')
  await delay(10)
  expect(inputs(f)).toHaveLength(2)
})

test('ambiguous HTTP retry preserves the admission identity and original context', async () => {
  const f = await fixture({ lostResponse: true })
  teardown(f, ask(f))
  f.speak('Yeah', 'retry-turn')
  await delay(20)
  expect(inputs(f)).toHaveLength(2)
  expect(inputs(f)[1].body).toEqual(inputs(f)[0].body)
  f.delegate('Yeah', 'real-route', 'retry-turn')
  await delay(10)
  expect(inputs(f)).toHaveLength(2)
})

test('an unadmitted reply leaves the question available for a newer user turn', async () => {
  const f = await fixture({ rejectedInput: true })
  teardown(f, ask(f))
  f.speak('Yeah', 'rejected-turn')
  await delay(10)
  expect(inputs(f)).toHaveLength(1)
  expect(f.snapshot.messages).toHaveLength(0)
  f.speak('Yeah', 'newer-turn')
  await delay(10)
  expect(inputs(f)).toHaveLength(2)
  expect(f.snapshot.messages).toHaveLength(1)
  expect(inputs(f)[1].body.contexts[0].request_id).toBe('newer-turn')
  expect(inputs(f)[1].body.contexts[0].text).toContain('Do you mean Hadrian?')
})

test('public deltas and a metadata-only delegation admit immediately while a question is pending', async () => {
  const f = await fixture()
  ask(f)
  f.emit({ type: 'session.input_transcript.delta', delta: 'Yeah', start_ms: 100 })
  f.emit({ type: 'session.delegation.created', offset_ms: 200, delegation: { id: 'public-first', target: 'client' } })
  await delay(10)
  expect(inputs(f)).toHaveLength(1)
  expect(inputs(f)[0].body.message).toBe('Yeah')
  expect(inputs(f)[0].body.contexts[0].request_id).toBe('public-first')
  expect(inputs(f)[0].body.contexts[0].text).toContain('Do you mean Hadrian?')
  f.emit({ type: 'session.output_transcript.delta', delta: 'Understood', start_ms: 210 })
  f.emit({ type: 'session.input_transcript.delta', delta: 'What is the time?', start_ms: 300 })
  f.emit({ type: 'session.delegation.created', offset_ms: 400, delegation: { id: 'public-second', target: 'client' } })
  f.emit({ type: 'session.delegation.created', offset_ms: 400, delegation: { id: 'public-second', target: 'client' } })
  await delay(10)
  expect(inputs(f)).toHaveLength(2)
  expect(inputs(f)[1].body.message).toBe('What is the time?')
  expect(inputs(f)[1].body.contexts[0].request_id).toBe('public-second')
})

test('captured second delegation for one user turn is ignored after idle and late bot output is still forwarded', async () => {
  const f = await fixture()
  const text = duplicateTurns[0].item.content[0].text
  f.speak(text, duplicateTurns[0].item.user_bidi_turn_id)
  f.emit(duplicateTurns[0])
  await delay(10)
  const answer = 'I only searched memory and asked a clarification. I had not started a research thread.'
  f.agent({ type: 'room_message', room_message: { speaker: 'bot', bot_id: 'thread', text: answer } })
  await delay(10)
  f.snapshot.session.status = 'idle'
  f.snapshot.acp_state = 'idle'
  f.agent({ type: 'acp', acp: { id: 'thread', state: 'idle' } })
  await delay(10)
  f.emit(duplicateTurns[1])
  await delay(10)
  expect(f.requests.filter((request) => request.path.endsWith('/agent/input'))).toHaveLength(1)
  expect(f.texts()).not.toContain('finished without a written answer')
  f.agent({ type: 'room_message', seq: 7, room_message: { speaker: 'bot', bot_id: 'thread', text: answer } })
  await delay(10)
  expect(f.speakable().match(/I only searched memory/g)).toHaveLength(2)
  f.agent({ type: 'acp', seq: 8, acp: { id: 'thread', state: 'idle' } })
  await delay(10)
  expect(f.speakable().match(/I only searched memory/g)).toHaveLength(2)
  // The same words in a genuinely new turn must still be admitted.
  f.speak(text, 'different-user-turn')
  f.emit({ ...duplicateTurns[1], item: { ...duplicateTurns[1].item, user_bidi_turn_id: 'different-user-turn' } })
  await delay(10)
  expect(f.requests.filter((request) => request.path.endsWith('/agent/input'))).toHaveLength(2)
})

test('OAuth acknowledgments correlate by delegation with one append in flight, even across out-of-order items', async () => {
  const f = await fixture({ autoAck: false })
  f.speak('Check memory')
  f.delegate('Check memory', 'first')
  await delay(10)
  f.speak('Check the tools too')
  f.delegate('Check the tools too', 'second')
  await delay(10)
  const first = f.wire.find((event) => event.delegation_item_id === 'first')
  const second = f.wire.find((event) => event.delegation_item_id === 'second')
  expect(f.wire.filter((event) => event.delegation_item_id === 'first')).toHaveLength(1)
  expect(f.wire.filter((event) => event.delegation_item_id === 'second')).toHaveLength(1)
  f.emit({ type: 'delegation.context.appended', delegation_item_id: 'second', start_ms: 100, end_ms: 200 })
  f.emit({ type: 'delegation.context.appended', delegation_item_id: 'first', start_ms: 200, end_ms: 300 })
  await delay(300)
  const records = f.requests.filter((request) => request.path.endsWith('/voice/diagnostics')).flatMap((request) => request.body)
  const acks = records.filter((record) => record.stage === 'append_ack')
  expect(acks.map((record) => record.event_id)).toEqual([second.event_id, first.event_id])
  expect(acks.map((record) => record.request_id)).toEqual(['second', 'first'])
})

test('a disconnected result channel reports delivery failure once and keeps the reply in chat', async () => {
  const f = await fixture()
  f.speak('Check memory')
  f.delegate('Check memory', 'first')
  await delay(10)
  f.channel.readyState = 'closed'
  f.agent({ type: 'room_message', room_message: { speaker: 'bot', bot_id: 'thread', text: 'Only memory_search was called.' } })
  await delay(10)
  expect(f.states.filter((state) => state.phase === 'error')).toHaveLength(1)
  expect(f.states.at(-1).error).toContain('remains available in the chat')
  expect(f.snapshot.events.at(-1).room_message.text).toBe('Only memory_search was called.')
})
