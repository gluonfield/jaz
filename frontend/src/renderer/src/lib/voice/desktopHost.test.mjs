import { afterEach, expect, test } from 'bun:test'
import { build } from 'bun'
import { setTimeout as delay } from 'node:timers/promises'

const bundle = await build({ entrypoints: [new URL('./desktopHost.ts', import.meta.url).pathname], target: 'browser', format: 'cjs', define: { 'import.meta.env': '{}' } })
if (!bundle.success) {
  throw new AggregateError(bundle.logs)
}
const compiled = new Function('module', 'exports', await bundle.outputs[0].text())
const originals = new Map()
let stop

function replace(name, value) {
  originals.set(name, Object.getOwnPropertyDescriptor(globalThis, name))
  Object.defineProperty(globalThis, name, { configurable: true, writable: true, value })
}

afterEach(() => {
  stop?.()
  stop = undefined
  for (const [name, descriptor] of originals) {
    if (descriptor) {
      Object.defineProperty(globalThis, name, descriptor)
    } else {
      delete globalThis[name]
    }
  }
  originals.clear()
})

function fixture() {
  const sockets = []
  const active = new Set(['desktop-local-call'])
  const closed = []
  const results = []
  let listener
  replace('window', { jaz: { apiBaseUrl: 'https://voice.invalid', windowKind: 'main' } })
  replace('navigator', { platform: 'MacIntel' })
  replace('WebSocket', class extends EventTarget {
    static OPEN = 1
    readyState = 1
    sent = []
    constructor(url) {
      super()
      expect(url).toBe('wss://voice.invalid/v1/voice/desktop')
      sockets.push(this)
    }
    send(data) {
      this.sent.push(JSON.parse(data))
    }
    close() {
      if (this.readyState === 3) {
        return
      }
      this.readyState = 3
      this.dispatchEvent(new Event('close'))
      this.onclose?.()
    }
  })
  const api = {
    status: async () => ({ available: true }),
    start: async (id, options) => {
      expect(options).toEqual({ sdp: 'v=0\r\noffer', context: 'Canonical instructions', voice: 'buttery' })
      active.add(id)
      return 'v=0\r\nanswer'
    },
    send: async (id, data) => results.push({ id, data }),
    result: async (id, toolUseId, text, isError) => results.push({ id, toolUseId, text, isError }),
    close: async (id) => {
      active.delete(id)
      closed.push(id)
    },
    onEvent: (handler) => {
      listener = handler
      return () => {
        listener = undefined
      }
    },
  }
  const module = { exports: {} }
  compiled(module, module.exports)
  stop = module.exports.connectClaudeVoiceHost(api)
  const command = async (socket, id, value) => socket.onmessage({ data: JSON.stringify({ id, method: 'Claude.voice', params: value }) })
  const start = (socket, id) => command(socket, id, { type: 'start', id, options: { sdp: 'v=0\r\noffer', context: 'Canonical instructions', voice: 'buttery' } })
  return { sockets, active, closed, results, command, start, emit: (id, data) => listener?.(id, data) }
}

test('desktop host routes two remote calls while preserving unrelated desktop-local voice', async () => {
  const f = fixture()
  const socket = f.sockets[0]
  await f.command(socket, 1, { type: 'status' })
  expect(socket.sent.at(-1)).toEqual({ id: 1, result: { available: true } })
  await f.start(socket, 'mobile-a')
  await f.start(socket, 'mobile-b')
  expect(socket.sent.at(-1)).toEqual({ id: 'mobile-b', result: 'v=0\r\nanswer' })
  f.emit('desktop-local-call', 'private desktop event')
  f.emit('mobile-a', 'only A')
  f.emit('mobile-b', 'only B')
  expect(socket.sent.slice(-2)).toEqual([
    { method: 'Claude.voiceEvent', params: { id: 'mobile-a', data: 'only A' } },
    { method: 'Claude.voiceEvent', params: { id: 'mobile-b', data: 'only B' } },
  ])
  expect(JSON.stringify(socket.sent)).not.toContain('private desktop event')
  await f.command(socket, 2, { type: 'result', id: 'mobile-b', toolUseId: 'tool-b', text: 'Accepted', isError: false })
  expect(f.results.at(-1)).toEqual({ id: 'mobile-b', toolUseId: 'tool-b', text: 'Accepted', isError: false })
  await f.command(socket, 3, { type: 'close', id: 'mobile-a' })
  expect(f.active).toEqual(new Set(['desktop-local-call', 'mobile-b']))
  f.emit('mobile-a', 'late A event')
  expect(JSON.stringify(socket.sent)).not.toContain('late A event')
  socket.close()
  expect(f.closed).toEqual(['mobile-a', 'mobile-b'])
  expect(f.active).toEqual(new Set(['desktop-local-call']))
})

test('reconnecting desktop cannot address calls from an earlier relay connection', async () => {
  const f = fixture()
  const original = f.sockets[0]
  await f.start(original, 'previous-mobile-call')
  original.close()
  await delay(2050)
  expect(f.sockets).toHaveLength(2)
  const next = f.sockets[1]
  await f.command(next, 2, { type: 'send', id: 'previous-mobile-call', data: '{"type":"observation"}' })
  expect(next.sent.at(-1).error.message).toContain('not owned')
  expect(f.results).toEqual([])
  expect(f.active).toEqual(new Set(['desktop-local-call']))
})
