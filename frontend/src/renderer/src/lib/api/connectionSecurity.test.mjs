import { afterEach, expect, test } from 'bun:test'
import { build } from 'bun'

const bundle = await build({
  entrypoints: ['connection-security-fixture'],
  target: 'browser',
  format: 'cjs',
  define: { 'import.meta.env': '{}' },
  plugins: [{
    name: 'security-entry',
    setup(builder) {
      builder.onResolve({ filter: /^connection-security-fixture$/ }, () => ({ path: 'fixture', namespace: 'security' }))
      builder.onResolve({ filter: /^\//, namespace: 'security' }, (args) => ({ path: args.path, namespace: 'file' }))
      builder.onLoad({ filter: /.*/, namespace: 'security' }, () => ({
        contents: `export * from ${JSON.stringify(new URL('./client.ts', import.meta.url).pathname)}\nexport * from ${JSON.stringify(new URL('./eventStream.ts', import.meta.url).pathname)}`,
        loader: 'ts',
      }))
    },
  }],
})
if (!bundle.success) {
  throw new AggregateError(bundle.logs)
}
const evaluate = new Function('module', 'exports', await bundle.outputs[0].text())
const originals = new Map()
let closeStream

function replace(name, value) {
  if (!originals.has(name)) {
    originals.set(name, Object.getOwnPropertyDescriptor(globalThis, name))
  }
  Object.defineProperty(globalThis, name, { configurable: true, writable: true, value })
}

afterEach(() => {
  closeStream?.()
  closeStream = undefined
  for (const [name, descriptor] of originals) {
    if (descriptor) {
      Object.defineProperty(globalThis, name, descriptor)
    } else {
      delete globalThis[name]
    }
  }
  originals.clear()
})

function fixture(base = 'https://jaz.example', jaz) {
  const stored = new Map()
  const sockets = []
  replace('window', { location: { origin: 'https://web.jaz.chat' }, jaz })
  replace('localStorage', {
    getItem: (key) => stored.get(key) ?? null,
    setItem: (key, value) => stored.set(key, value),
    removeItem: (key) => stored.delete(key),
  })
  replace('WebSocket', class {
    constructor(url, protocols) {
      sockets.push({ url, protocols })
      this.addEventListener = () => {}
      this.close = () => {}
    }
  })
  const module = { exports: {} }
  evaluate(module, module.exports)
  const api = module.exports
  api.setApiBaseUrl(base)
  api.setApiAuthToken(base, 'bearer-credential')
  return { api, sockets }
}

test('browser transports keep credentials in headers and bound URLs to read tickets', async () => {
  const { api, sockets } = fixture()
  let request
  replace('fetch', async (url, init) => {
    request = { url, init }
    return Response.json({ ok: true })
  })
  await api.apiFetch('/v1/auth/check')
  expect(request.init.headers.get('Authorization')).toBe('Bearer bearer-credential')
  api.setResourceTicket(api.apiBaseUrl(), { token: 'read-ticket', expires_at: new Date(Date.now() + 60_000).toISOString() })
  const embedded = new URL(api.apiEmbeddedGetUrl('/v1/sessions/thread/file?raw=1'))
  expect(embedded.searchParams.get('ticket')).toBe('read-ticket')
  expect(embedded.toString()).not.toContain('bearer-credential')
  api.apiWebSocket('/v1/sessions/thread/terminal')
  expect(sockets).toEqual([{ url: 'wss://jaz.example/v1/sessions/thread/terminal', protocols: ['jaz', 'jaz-auth.bearer-credential'] }])
  api.setApiAuthToken(api.apiBaseUrl(), null)
  expect(api.apiEmbeddedGetUrl('/v1/sessions/thread/file?raw=1')).not.toContain('ticket=')
})

test('HTTP remote addresses fail before any credential-bearing network call', () => {
  const { api, sockets } = fixture('http://jaz.example')
  let requests = 0
  replace('fetch', () => {
    requests += 1
    throw new Error('Network must remain unused')
  })
  expect(() => api.apiFetch('/v1/auth/check')).toThrow('HTTPS')
  expect(() => api.apiWebSocket('/v1/sessions/thread/terminal')).toThrow('HTTPS')
  expect(() => api.apiEmbeddedGetUrl('/v1/sessions/thread/file?raw=1')).toThrow('HTTPS')
  expect(requests).toBe(0)
  expect(sockets).toHaveLength(0)
})

test('event reconnection renews the read ticket and resumes from the last delivered sequence', async () => {
  const { api } = fixture()
  const sources = []
  const received = []
  let retry
  let requests = 0
  replace('setTimeout', (callback) => {
    retry = callback
    return 1
  })
  replace('clearTimeout', () => {})
  replace('fetch', async (_url, init) => {
    expect(init.headers.get('Authorization')).toBe('Bearer bearer-credential')
    requests += 1
    return Response.json({ resource_ticket: { token: `ticket-${requests}`, expires_at: new Date(Date.now() + 60_000).toISOString() } })
  })
  replace('EventSource', class {
    constructor(url) {
      this.url = new URL(url)
      this.handlers = new Map()
      sources.push(this)
    }
    addEventListener(name, handler) {
      this.handlers.set(name, handler)
    }
    close() {
      this.closed = true
    }
  })
  const settle = async () => {
    for (let i = 0; i < 20; i += 1) {
      await Promise.resolve()
    }
  }
  closeStream = api.openEventStream('/v1/sessions/thread/events?after_seq=4', { message: (event) => received.push(event.data) })
  await settle()
  expect(sources[0].url.searchParams.get('ticket')).toBe('ticket-1')
  sources[0].handlers.get('message')({ data: 'delivered', lastEventId: '7' })
  sources[0].onerror()
  expect(sources[0].closed).toBe(true)
  retry()
  await settle()
  expect(sources[1].url.searchParams.get('ticket')).toBe('ticket-2')
  expect(sources[1].url.searchParams.get('after_seq')).toBe('7')
  expect(received).toEqual(['delivered'])
  closeStream()
  expect(sources[1].closed).toBe(true)
})

test('late same-origin ticket refresh cannot replace a new credential or reopen a cleared stream', async () => {
  const { api } = fixture()
  let respond
  let requested
  const pending = new Promise((resolve) => { requested = resolve })
  replace('fetch', () => {
    requested()
    return new Promise((resolve) => { respond = resolve })
  })
  let sources = 0
  replace('EventSource', class { constructor() { sources += 1 } })
  closeStream = api.openEventStream('/v1/bots/events', {})
  await pending
  api.setApiAuthToken(api.apiBaseUrl(), 'replacement', { token: 'replacement-ticket', expires_at: '2099-01-01T00:00:00Z' })
  respond(Response.json({ resource_ticket: { token: 'old-ticket', expires_at: '2099-01-01T00:00:00Z' } }))
  for (let i = 0; i < 40; i += 1) await Promise.resolve()
  expect(sources).toBe(0)
  expect(api.apiEmbeddedGetUrl('/v1/sessions/thread/file?raw=1')).toContain('replacement-ticket')
  api.clearApiSession()
  expect(api.apiEmbeddedGetUrl('/v1/sessions/thread/file?raw=1')).not.toContain('ticket=')
})

test('session clear aborts an authenticated response body after its headers arrive', async () => {
  const { api } = fixture()
  replace('fetch', async (_url, init) => new Response(new ReadableStream({
    start(controller) {
      init.signal.addEventListener('abort', () => controller.error(new Error('Session ended')), { once: true })
    },
  })))
  const response = await api.apiFetch('/v1/sessions/thread/messages')
  const body = response.text().catch((error) => error)
  api.clearApiSession()
  expect(await body).toBeInstanceOf(Error)
})

test('write acknowledgements keep their null-body status when Chromium exposes an empty body', async () => {
  const { api } = fixture()
  replace('fetch', async () => Object.defineProperty(new Response(null, { status: 204 }), 'body', { value: new ReadableStream() }))
  expect(await api.post('/v1/sessions/thread/voice/transcript', [])).toBeUndefined()
})

test('assertBackendRelativePath and apiFetch reject protocol-relative and backslash paths before attaching Authorization', () => {
  const { api } = fixture()
  let hits = 0
  replace('fetch', async () => {
    hits += 1
    return Response.json({ ok: true })
  })
  for (const badPath of ['//evil.example/v1/steal', '/\\evil.example/v1/steal', 'https://evil.example/v1/steal', '/v1/a\\b']) {
    expect(() => api.apiFetch(badPath)).toThrow('backend-relative')
    expect(() => api.apiUrl(badPath)).toThrow('backend-relative')
    expect(() => api.apiEmbeddedGetUrl(badPath)).toThrow('backend-relative')
  }
  expect(hits).toBe(0)
})

test('parseBackendConnectUrl reads credentials only from fragment and ignores query params', () => {
  const { api } = fixture()
  expect(api.parseBackendConnectUrl('https://jaz.example/?token=leaked-query&key=leaked-key')).toEqual({
    url: 'https://jaz.example',
    key: '',
  })
  expect(api.parseBackendConnectUrl('https://jaz.example/#key=frag-secret')).toEqual({
    url: 'https://jaz.example',
    key: 'frag-secret',
  })
})
