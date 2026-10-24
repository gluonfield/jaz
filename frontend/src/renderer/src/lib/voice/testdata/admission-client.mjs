import assert from 'node:assert/strict'
import { setTimeout as delay } from 'node:timers/promises'
import { build } from 'bun'

const origin = process.env.JAZ_VOICE_TEST_ORIGIN
const sessionId = process.env.JAZ_VOICE_TEST_SESSION
const scenario = process.env.JAZ_VOICE_TEST_SCENARIO
const text = process.env.JAZ_VOICE_TEST_REPLY
assert.ok(origin?.startsWith('http://127.0.0.1:'))
const bundle = await build({ entrypoints: [new URL('../session.ts', import.meta.url).pathname], target: 'browser', format: 'cjs', define: { 'import.meta.env': '{"VITE_JAZ_API_URL":"origin"}' } })
assert.equal(bundle.success, true)
globalThis.window = { location: { origin } }
globalThis.localStorage = { getItem: () => null }
const module = { exports: {} }
new Function('module', 'exports', await bundle.outputs[0].text())(module, module.exports)
const { VoiceSession } = module.exports
const nativeFetch = globalThis.fetch
const inputs = []
const wire = []
let lost = true
let lastSeq = 0
const origins = []
let channel
globalThis.window = { location: { origin } }
globalThis.localStorage = { getItem: () => null }
Object.defineProperty(globalThis, 'navigator', { configurable: true, value: { platform: 'MacIntel', mediaDevices: { getUserMedia: async () => ({ getTracks: () => [], getAudioTracks: () => [] }) } } })
globalThis.Audio = class extends EventTarget {
  play() {
    throw new Error('Audio must remain isolated')
  }
  pause() {}
}
globalThis.AudioContext = class {
  createAnalyser() {
    return {}
  }
  createMediaStreamSource() {
    return { connect() {} }
  }
  async resume() {}
  async close() {}
}
globalThis.RTCPeerConnection = class {
  iceGatheringState = 'complete'
  createDataChannel() {
    channel = {
      readyState: 'open',
      close() {},
      send(data) {
        const event = JSON.parse(data)
        wire.push(event)
        if (event.type.endsWith('context.append')) {
          queueMicrotask(() => channel.onmessage({ data: JSON.stringify({ type: `${event.type}ed`, delegation_item_id: event.delegation_item_id }) }))
        }
      },
    }
    return channel
  }
  addTrack() {}
  async createOffer() {
    return { type: 'offer', sdp: 'local-offer' }
  }
  async setLocalDescription(value) {
    this.localDescription = value
  }
  async setRemoteDescription() {
    queueMicrotask(() => channel.onmessage({ data: '{"type":"session.started"}' }))
  }
  close() {}
}
globalThis.EventSource = class {
  addEventListener(name, handler) {
    this["on" + name] = handler
  }
  abort = new AbortController()
  constructor(url) {
    assert.equal(new URL(url).origin, origin, 'SSE must stay on the fixture server')
    queueMicrotask(async () => {
      try {
        const response = await nativeFetch(url, { signal: this.abort.signal })
        this.onopen?.()
        let buffer = ''
        for await (const chunk of response.body) {
          buffer += new TextDecoder().decode(chunk)
          let index
          while ((index = buffer.indexOf('\n\n')) >= 0) {
            const block = buffer.slice(0, index)
            buffer = buffer.slice(index + 2)
            const data = block.split('\n').filter((line) => line.startsWith('data:')).map((line) => line.slice(5).trimStart()).join('\n')
            if (data) {
              const event = JSON.parse(data)
              lastSeq = Math.max(lastSeq, event.seq ?? 0)
              if (event.acp?.origin) {
                origins.push(event.acp.origin)
              }
              if (event.permission?.origin) {
                origins.push(event.permission.origin)
              }
              this.onmessage?.({ data })
            }
          }
        }
      } catch (error) {
        if (!this.abort.signal.aborted) {
          throw error
        }
      }
    })
  }
  close() {
    this.abort.abort()
  }
}
globalThis.fetch = async (url, init) => {
  assert.equal(new URL(url).origin, origin, 'HTTP must stay on the fixture server')
  const path = new URL(url).pathname
  if (path.endsWith('/connect')) {
    return Response.json({ provider: 'openai', sdp: 'local-answer' })
  }
  if (path.endsWith('/voice/transcript') || path.endsWith('/voice/diagnostics')) {
    return new Response(null, { status: 204 })
  }
  const response = await nativeFetch(url, init)
  if (path.endsWith('/agent/input')) {
    inputs.push(JSON.parse(init.body))
    assert.equal(response.status, 200, await response.clone().text())
    if (lost) {
      lost = false
      throw new TypeError('Simulated response loss after the real backend accepted input')
    }
  }
  assert.ok(!path.endsWith('/interactive-response'), 'Speech must use ordinary input')
  return response
}
async function waitFor(check, label) {
  for (let attempt = 0; attempt < 400; attempt++) {
    if (check()) return
    await delay(10)
  }
  throw new Error(`Timed out waiting for ${label}`)
}
async function start() {
  let state
  const session = new VoiceSession(sessionId, (value) => { state = value }, () => {})
  session.start()
  await waitFor(() => {
    if (state?.error) {
      throw new Error(`Voice startup: ${state.error}`)
    }
    return session.phase === 'listening'
  }, 'voice startup')
  return session
}
let session = await start()
try {
  if (scenario !== 'pending') {
    const finished = await (await nativeFetch(`${origin}/fixture/finish`)).json()
    await waitFor(() => lastSeq >= finished.seq, 'actual teardown SSE')
  }
  if (['remount', 'user_stop', 'unknown'].includes(scenario)) {
    session.dispose()
    session = await start()
  }
  const emit = (event) => channel.onmessage({ data: JSON.stringify(event) })
  emit({ type: 'turn.created', turn: { id: 'reply-turn', role: 'user' } })
  emit({ type: 'turn.done', turn: { id: 'reply-turn', role: 'user', transcript: text } })
  if (scenario === 'user_stop') {
    await delay(100)
    assert.equal(inputs.length, 0, 'Explicit Stop must survive storage, later status, compaction and remount')
    assert.ok(origins.includes('user_stop'), 'The real Stop origin must cross SSE')
    console.log(JSON.stringify({ scenario, inputs, origins }))
    session.dispose()
    process.exit(0)
  }
  await waitFor(() => inputs.length === 2 && wire.some((event) => event.content?.[0]?.text.startsWith('Agent request accepted:')), 'ordinary admission and response retry')
  assert.deepEqual(inputs[1], inputs[0], 'A retry must preserve call + native turn and context')
  assert.equal(inputs[0].message, text)
  assert.equal(inputs[0].contexts[0].request_id, 'reply-turn')
  assert.match(inputs[0].contexts[0].text, /Do you mean Hadrian/)
  assert.match(inputs[0].contexts[0].text, /turn_started_at/)
  if (scenario !== 'pending') assert.match(inputs[0].contexts[0].text, scenario === 'unknown' ? /unknown/ : /turn_cleanup/)
  assert.ok(wire.every((event) => !event.delegation_item_id), 'Transcript admission must have no fabricated delegation route')
  for (const id of ['real-item-1', 'real-item-2']) {
    emit({ type: 'delegation.created', item: { id, target: 'client', user_bidi_turn_id: 'reply-turn', content: [{ type: 'input_text', text }] } })
  }
  await delay(50)
  assert.equal(inputs.length, 2, 'Both real delegations must attach to the admitted native turn')
  if (scenario === 'pending') await nativeFetch(`${origin}/fixture/finish`)
  console.log(JSON.stringify({ scenario, inputs, lastSeq }))
} finally {
  session.dispose()
}
