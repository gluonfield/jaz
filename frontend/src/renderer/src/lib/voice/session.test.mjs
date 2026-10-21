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
  originals.set(name, Object.getOwnPropertyDescriptor(globalThis, name))
  Object.defineProperty(globalThis, name, { configurable: true, writable: true, value })
}

afterEach(() => {
  session?.dispose()
  for (const [name, descriptor] of originals) {
    if (descriptor) {
      Object.defineProperty(globalThis, name, descriptor)
    } else {
      delete globalThis[name]
    }
  }
  originals.clear()
})

async function fixture({ staleAnswer = false, autoAck = true } = {}) {
  const requests = []
  const wire = []
  const states = []
  const snapshot = { session: { id: 'thread', title: 'Prime', status: 'running', source_type: 'bot' }, messages: [], events: [], acp_state: 'running', acp_permissions: [], latest_event_seq: 0 }
  const subscriptions = []
  let seq = 0
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
      const context = body.contexts[0]
      snapshot.messages.push({ seq: snapshot.messages.length + 1, role: 'user', content: body.message, created_at: new Date().toISOString(), blocks: [{ type: 'voice_context', id: context.id, request_id: context.request_id }] })
    } else if (path.endsWith('/interactive-response')) {
      const pending = snapshot.acp_permissions.find((permission) => permission.id === body.request_id)
      expect(pending.status).toBe('pending')
      expect(body.answers).toEqual({ manufacturer: { answers: ['Yeah'] } })
      if (staleAnswer) {
        pending.status = 'cancelled'
        snapshot.acp_state = 'idle'
        snapshot.session.status = 'idle'
        return Response.json({ error: 'pending permission request not found' }, { status: 400 })
      }
      pending.status = 'selected'
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
  const speak = (text) => {
    emit({ type: 'input_transcript.added', item: { text } })
    emit({ type: 'turn.done', turn: { role: 'user', transcript: text } })
  }
  const agent = (event) => {
    const record = { session_id: 'thread', at: new Date().toISOString(), seq: ++seq, ...event }
    snapshot.events.push(record)
    snapshot.latest_event_seq = record.seq
    subscriptions.at(-1).onmessage({ data: JSON.stringify(record) })
  }
  const texts = () => wire.map((event) => event.content?.[0]?.text ?? '').join('')
  const speakable = () => wire.filter((event) => event.channel === 'speakable').map((event) => event.content[0].text).join('')
  const delegate = (text, id = crypto.randomUUID()) => emit({ type: 'delegation.created', item: { id, target: 'client', content: [{ type: 'input_text', text }] } })
  return { requests, wire, states, snapshot, channel, emit, speak, agent, texts, speakable, delegate }
}

const answerChoice = JSON.stringify({ type: 'answer_question', request_id: 'question', question_id: 'manufacturer', answer: 'Yeah' })

test('recorded clarification and interruption script uses the real voice session and task stream silently', async () => {
  const f = await fixture()
  f.speak('Who manufactures parts like Adrian?')
  const delegation = { type: 'delegation.created', item: { id: 'research', target: 'client', content: [{ type: 'input_text', text: 'Who manufactures parts like Adrian?' }] } }
  f.emit(delegation)
  f.emit(delegation)
  await delay(10)
  expect(f.requests.filter((request) => request.path.endsWith('/agent/input'))).toHaveLength(1)
  const accepted = f.wire.find((event) => event.content?.[0]?.text.startsWith('Agent request accepted:'))
  expect(accepted.channel).toBe('commentary')
  expect(accepted.event_id).toBeTruthy()
  f.agent({ type: 'acp_tool', acp: { id: 'thread', state: 'running', tool_calls: [{ id: 'lookup', title: 'memory_search', status: 'completed', raw_output: { private: 'never say this' } }] } })
  expect(f.texts()).toContain('memory_search: completed')
  expect(f.texts()).not.toContain('never say this')
  f.agent({ type: 'room_message', room_message: { speaker: 'bot', bot_id: 'thread', text: 'Do you mean Hadrian, and other companies like it that manufacture parts?' } })
  expect(f.speakable()).toContain('Do you mean Hadrian')
  const permission = { id: 'question', status: 'pending', title: 'Questions', questions: [{ id: 'manufacturer', question: 'Do you mean Hadrian?' }] }
  f.snapshot.acp_permissions = [permission]
  f.agent({ type: 'permission_request', permission })
  await delay(10)
  expect(f.texts()).toContain("Waiting for the user's answer: Do you mean Hadrian?")
  f.speak('Yeah')
  expect(f.requests.filter((request) => request.path.endsWith('/interactive-response'))).toHaveLength(0)
  f.delegate(answerChoice, 'answer')
  await delay(10)
  expect(f.requests.filter((request) => request.path.endsWith('/interactive-response'))).toHaveLength(1)
  expect(permission.status).toBe('selected')
  f.delegate(answerChoice, 'late-duplicate')
  await delay(10)
  expect(f.requests.filter((request) => request.path.endsWith('/agent/input'))).toHaveLength(1)
  f.emit({ type: 'output_transcript.added', item: { text: "I'll pull that together" } })
  f.speak('Wait, did you spawn an agent?')
  expect(f.states.at(-1).activity).toBe('working')
  f.agent({ type: 'room_message', room_message: { speaker: 'bot', bot_id: 'thread', text: 'I only searched memory and asked a clarification. I had not started a research thread.' } })
  expect(f.speakable()).toContain('I only searched memory')
  expect(f.snapshot.acp_state).toBe('running')
  f.speak("You should know what you did. Didn't you literally call the tool?")
  const before = f.speakable()
  await delay(250)
  f.agent({ type: 'acp', acp: { id: 'thread', state: 'running' } })
  expect(f.speakable()).toBe(before)
  f.agent({ type: 'acp', acp: { id: 'thread', state: 'idle' } })
  const after = f.speakable()
  expect(after.match(/I only searched memory/g)).toHaveLength(1)
  expect(f.states.at(-1).activity).toBe(null)
})

test.each(['Before that, did you spawn an agent?', 'Stop working on this'])('pending clarification preserves ordinary admission: %s', async (text) => {
    const f = await fixture()
    f.snapshot.acp_permissions = [{ id: 'question', status: 'pending', questions: [{ id: 'manufacturer', question: 'Do you mean Hadrian?' }] }]
    f.agent({ type: 'permission_request', permission: f.snapshot.acp_permissions[0] })
    await delay(10)
    f.speak(text)
    f.delegate(text)
    await delay(10)
    expect(f.requests.filter((request) => request.path.endsWith('/interactive-response'))).toHaveLength(0)
    expect(f.requests.filter((request) => request.path.endsWith('/agent/input')).map((request) => request.body.message)).toEqual([text])
})

test('a question cancelled between snapshot and POST falls back once with the original context', async () => {
  const f = await fixture({ staleAnswer: true })
  f.snapshot.acp_permissions = [{ id: 'question', status: 'pending', questions: [{ id: 'manufacturer', question: 'Do you mean Hadrian?' }] }]
  f.agent({ type: 'permission_request', permission: f.snapshot.acp_permissions[0] })
  await delay(10)
  f.speak('Yeah')
  f.delegate(answerChoice, 'late-answer')
  f.delegate(answerChoice, 'late-answer')
  await delay(20)
  const inputs = f.requests.filter((request) => request.path.endsWith('/agent/input'))
  expect(f.requests.filter((request) => request.path.endsWith('/interactive-response'))).toHaveLength(1)
  expect(inputs).toHaveLength(1)
  expect(inputs[0].body.message).toBe('Yeah')
  expect(inputs[0].body.contexts[0].text).toBe('User: Yeah')
})

test('ordinary acknowledgements and ambiguous multiple questions never become automatic answers', async () => {
  const f = await fixture()
  f.speak('Yeah')
  f.snapshot.acp_permissions = [{ id: 'many', status: 'pending', questions: [{ id: 'one', question: 'Which company?' }, { id: 'two', question: 'Which country?' }] }]
  f.agent({ type: 'permission_request', permission: f.snapshot.acp_permissions[0] })
  await delay(10)
  f.speak('Yeah')
  await delay(10)
  expect(f.requests.some((request) => request.path.endsWith('/interactive-response') || request.path.endsWith('/agent/input'))).toBe(false)
  expect(f.texts()).not.toContain('Agent work completed')
})


test('captured second delegation for one user turn is ignored after idle and late bot output is still forwarded', async () => {
  const f = await fixture()
  const text = duplicateTurns[0].item.content[0].text
  f.speak(text)
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
  f.speak(text)
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
