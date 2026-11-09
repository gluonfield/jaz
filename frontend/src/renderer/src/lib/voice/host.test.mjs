import { afterEach, expect, test } from 'bun:test'
import { build } from 'bun'
import { setTimeout as delay } from 'node:timers/promises'

const bundle = await build({ entrypoints: [new URL('./session.ts', import.meta.url).pathname], target: 'browser', format: 'cjs' })
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

function fixture(provider, nativeSink = true, answer = 'v=0\r\nanswer', { settingsProvider = provider, responseProvider = provider } = {}) {
  const states = []
  const transcripts = []
  const batches = []
  const inputs = []
  const connections = []
  const starts = []
  const descriptions = []
  const peers = []
  const closed = []
  const track = { enabled: true, stopped: false, stop() { this.stopped = true } }
  const snapshot = { session: { id: 'thread', title: 'Voice', status: 'running' }, messages: [], events: [], latest_event_seq: 0, acp_state: 'running' }
  let listener
  let callId
  const channel = {
    readyState: 'open',
    send(data) {
      const event = JSON.parse(data)
      const type = event.type === 'session.close' ? 'session.closed' : event.type + 'ed'
      queueMicrotask(() => this.onmessage({ data: JSON.stringify({ type }) }))
    },
    close() { this.readyState = 'closed' },
  }
  replace('window', undefined)
  Object.defineProperty(globalThis, 'window', { configurable: true, get() { throw new Error('The shared engine cannot use desktop globals') } })
  replace('fetch', () => { throw new Error('The shared engine must use its host transport') })
  replace('Audio', class extends EventTarget {
    async play() {}
    pause() {}
  })
  replace('AudioContext', class {
    createAnalyser() { return {} }
    createMediaStreamSource() { return { connect() {} } }
    async resume() {}
    async close() {}
  })
  replace('RTCPeerConnection', class {
    constructor() {
      this.closed = false
      peers.push(this)
    }
    iceGatheringState = 'complete'
    createDataChannel() {
      expect(provider).not.toBe('claude')
      return channel
    }
    addTrack() {}
    async createOffer() { return { type: 'offer', sdp: 'v=0\r\noffer' } }
    async setLocalDescription(offer) { this.localDescription = offer }
    async setRemoteDescription(description) {
      descriptions.push(description)
      if (provider !== 'claude') {
        queueMicrotask(() => channel.onmessage({ data: '{"type":"session.started"}' }))
      }
    }
    close() { this.closed = true }
  })
  const host = {
    getSettings: async () => ({ agent: provider === 'claude' ? 'claude' : 'openai', provider: settingsProvider, voice: 'fixture', voices: ['fixture'], providers: [{ id: provider, available: true }] }),
    connect: async (sessionId, sdp, context, signal) => {
      connections.push({ sessionId, sdp, context, signal })
      return provider === 'claude' ? { provider: responseProvider, instructions: 'Canonical provider instructions' } : { provider: responseProvider, model: 'fixture', sdp: answer }
    },
    readSession: async () => structuredClone(snapshot),
    subscribeSession: () => () => {},
    sendTask: async (sessionId, text, contexts) => {
      inputs.push({ sessionId, text, contexts })
      snapshot.messages.push({ id: crypto.randomUUID(), seq: snapshot.messages.length + 1, role: 'user', content: text, created_at: new Date().toISOString(), blocks: [{ type: 'voice_context', id: contexts[0].id, request_id: contexts[0].request_id }] })
    },
    saveDiagnostics: async () => {},
    requestMicrophone: async () => ({ getTracks: () => [track], getAudioTracks: () => [track] }),
    claude: {
      start: async (id, options, setup) => {
        callId = id
        starts.push({ options, setup })
        return answer
      },
      send: async () => {},
      result: async () => {},
      close: async (id) => { closed.push(id) },
      onEvent: (handler) => {
        listener = handler
        return () => { listener = undefined }
      },
    },
    ...(nativeSink ? { transcript: (message, final) => transcripts.push({ message, final }) } : { saveTranscript: async (_sessionId, messages) => batches.push(messages) }),
  }
  const module = { exports: {} }
  compiled(module, module.exports)
  session = new module.exports.VoiceSession('thread', (state) => states.push(state), () => {}, host)
  const emit = (event) => provider === 'claude' ? listener(callId, JSON.stringify(event)) : channel.onmessage({ data: JSON.stringify(event) })
  return { states, transcripts, batches, inputs, connections, starts, descriptions, peers, closed, track, emit }
}

test.each(['openai', 'claude'])('shared %s engine admits typed input with native transcript persistence and raw setup context', async (provider) => {
  const f = fixture(provider)
  expect(session.sendText('Keep draft')).toBe(false)
  session.start()
  await delay(10)
  expect(f.states.at(-1).phase).toBe('listening')
  if (provider === 'claude') {
    expect(f.starts[0].options.context).toBe('Canonical provider instructions')
    expect(f.starts[0].setup.context).toBe(f.connections[0].context)
    expect(f.starts[0].setup.context).not.toContain('Canonical provider instructions')
    expect(f.starts[0].setup.sessionId).toBe('thread')
    f.emit({ type: 'transcript_interim', text: 'Spoken draft' })
  } else {
    f.emit({ type: 'input_transcript.added', item: { text: 'Spoken draft' } })
  }
  const spoken = f.transcripts.at(-1).message
  expect(session.sendText('  Include tests  ')).toBe(true)
  const typed = f.transcripts.at(-1)
  expect(typed.final).toBe(true)
  expect(typed.message.text).toBe('Include tests')
  expect(typed.message.id).not.toBe(spoken.id)
  await delay(10)
  expect(f.inputs).toHaveLength(1)
  expect(f.inputs[0].contexts[0].text).toBe('User: Spoken draft\nUser: Include tests')
  expect(f.inputs[0].contexts[0].id).toBe(typed.message.call_id)
  if (provider === 'claude') {
    f.emit({ type: 'message_complete', data: { sender: 'human', message_uuid: 'spoken-turn', content: [{ type: 'text', text: 'Spoken final' }] } })
  } else {
    f.emit({ type: 'turn.done', turn: { role: 'user', transcript: 'Spoken final' } })
  }
  expect(f.transcripts.at(-1)).toEqual({ message: { ...spoken, text: 'Spoken final' }, final: true })
  expect(f.batches).toEqual([])
  session.end()
  await delay(5)
  expect(session.sendText('Keep draft')).toBe(false)
  expect(f.track.stopped).toBe(true)
  expect(f.states.at(-1).phase).toBe('off')
})

test('shared typed input uses the canonical writer when the host has no native sink', async () => {
  const f = fixture('openai', false)
  session.start()
  await delay(10)
  expect(session.sendText('Persist typed input')).toBe(true)
  await delay(10)
  expect(f.batches.flat().map((message) => message.text)).toEqual(['Persist typed input'])
  expect(f.transcripts).toEqual([])
})

test.each(['openai', 'claude'])('shared %s adapter rejects malformed SDP before WebRTC and releases capture', async (provider) => {
  const f = fixture(provider, true, '{"error":"not SDP"}')
  session.start()
  await delay(10)
  expect(f.states.at(-1)).toMatchObject({ phase: 'error', error: 'The voice provider returned an invalid connection answer. Please try again.' })
  expect(f.connections).toHaveLength(1)
  expect(f.starts).toHaveLength(provider === 'claude' ? 1 : 0)
  expect(f.descriptions).toEqual([])
  expect(f.peers).toHaveLength(1)
  expect(f.peers[0].closed).toBe(true)
  expect(f.track.stopped).toBe(true)
})


test.each([null, {}, 'unexpected'])('shared engine rejects runtime provider %j before selecting a transport', async (settingsProvider) => {
  const f = fixture('openai', true, 'v=0\r\nanswer', { settingsProvider })
  session.start()
  await delay(10)
  expect(f.states.at(-1)).toMatchObject({ phase: 'error', error: 'Unsupported voice provider. Choose a provider in Voice settings.' })
  expect(f.connections).toEqual([])
  expect(f.starts).toEqual([])
  expect(f.descriptions).toEqual([])
  expect(f.peers).toHaveLength(1)
  expect(f.peers[0].closed).toBe(true)
  expect(f.track.stopped).toBe(true)
})

test.each(['openai', 'claude'])('shared %s adapter rejects unknown response providers at the canonical boundary', async (provider) => {
  const f = fixture(provider, true, 'v=0\r\nanswer', { responseProvider: 'unknown' })
  session.start()
  await delay(10)
  expect(f.connections).toHaveLength(1)
  expect(f.states.at(-1)).toMatchObject({ phase: 'error', error: 'Unsupported voice provider. Choose a provider in Voice settings.' })
  expect(f.starts).toEqual([])
  expect(f.descriptions).toEqual([])
  expect(f.peers).toHaveLength(1)
  expect(f.peers[0].closed).toBe(true)
  expect(f.track.stopped).toBe(true)
})

test.each(['openai', 'openai-api-key'])('shared %s adapter rejects a switch between account and API connections', async (provider) => {
  const responseProvider = provider === 'openai' ? 'openai-api-key' : 'openai'
  const f = fixture(provider, true, 'v=0\r\nanswer', { responseProvider })
  session.start()
  await delay(10)
  expect(f.states.at(-1)).toMatchObject({ phase: 'error', error: 'Voice provider changed. Start voice again.' })
  expect(f.connections).toHaveLength(1)
  expect(f.starts).toEqual([])
  expect(f.descriptions).toEqual([])
  expect(f.peers).toHaveLength(1)
  expect(f.peers[0].closed).toBe(true)
  expect(f.track.stopped).toBe(true)
})
