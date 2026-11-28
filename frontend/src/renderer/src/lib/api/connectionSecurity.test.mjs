import { afterEach, expect, test } from 'bun:test'
import { build } from 'bun'
import * as ReactQuery from '@tanstack/react-query'
import * as React from 'react'

const bundle = await build({
  entrypoints: ['connection-security-fixture'],
  target: 'browser',
  format: 'cjs',
  external: ['react', '@tanstack/react-query', '@/lib/connection', '@/lib/cloudAccount'],
  define: { 'import.meta.env': '{}' },
  plugins: [{
    name: 'security-entry',
    setup(builder) {
      builder.onResolve({ filter: /^connection-security-fixture$/ }, () => ({ path: 'fixture', namespace: 'security' }))
      builder.onResolve({ filter: /^\//, namespace: 'security' }, (args) => ({ path: args.path, namespace: 'file' }))
      builder.onLoad({ filter: /.*/, namespace: 'security' }, () => ({
        contents: `export * from ${JSON.stringify(new URL('./client.ts', import.meta.url).pathname)}
export * from ${JSON.stringify(new URL('./eventStream.ts', import.meta.url).pathname)}
export { useBotEvents } from '@/lib/hooks/useBotEvents'
export { botMood } from '@/lib/bots'
export { keys } from '@/lib/query/keys'`,
        loader: 'ts',
      }))
    },
  }],
})
if (!bundle.success) {
  throw new AggregateError(bundle.logs)
}
const evaluate = new Function('require', 'module', 'exports', await bundle.outputs[0].text())
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
  const queryClient = new ReactQuery.QueryClient()
  const modules = {
    react: { ...React, useEffect: (effect) => { closeStream = effect() } },
    '@tanstack/react-query': { ...ReactQuery, useQueryClient: () => queryClient },
    '@/lib/connection': { useConnection: () => ({ status: 'connected', mode: module.exports.apiServerMode() }) },
    '@/lib/cloudAccount': { useCloudAccount: () => ({ activeWorkspaceId: 'local' }) },
  }
  evaluate((name) => modules[name], module, module.exports)
  const api = module.exports
  api.setApiBaseUrl(base)
  api.setApiAuthToken(base, 'bearer-credential')
  return { api, sockets, queryClient }
}

function captureEventSources() {
  const sources = []
  replace('EventSource', class {
    handlers = new Map()
    constructor(url) {
      this.url = new URL(url)
      sources.push(this)
    }
    addEventListener(name, handler) {
      this.handlers.set(name, handler)
    }
    close() {
      this.closed = true
    }
  })
  return sources
}

async function settle() {
  for (let i = 0; i < 20; i += 1) {
    await Promise.resolve()
  }
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
  const sources = captureEventSources()
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

test.each(['Desktop', 'Web'])('%s Cloud bot events use session auth and update mood and preview live', async (platform) => {
  const origin = 'https://cloud.example.com'
  const web = platform === 'Web'
  const { api, queryClient } = fixture(origin, web ? undefined : { apiBaseUrl: origin })
  api.setApiServerMode('cloud')
  if (web) api.setApiAuthToken(origin, null)
  const sources = captureEventSources()
  let retry
  replace('setTimeout', (callback) => {
    retry = callback
    return 1
  })
  replace('clearTimeout', () => {})
  const requests = []
  replace('fetch', async (url, init) => {
    requests.push([String(url), init.headers.get('Authorization'), init.credentials])
    const ticket = requests.length === 1
      ? { token: 'read-ticket-1', expires_at: '2099-01-01T00:00:00Z' }
      : 'read-ticket-2'
    return Response.json({
      user_id: 'user-1',
      org_id: 'org-1',
      resource_ticket: ticket,
    })
  })

  api.useBotEvents()
  await settle()
  const authorization = web ? null : 'Bearer bearer-credential'
  const credentials = web ? 'include' : undefined
  expect(requests).toEqual([[`${origin}/v1/auth/me`, authorization, credentials]])
  expect(sources[0]?.url.href).toBe(`${origin}/v1/bots/events?ticket=read-ticket-1`)

  const emit = (source, name, data) => source.handlers.get(name)({ data: JSON.stringify(data) })
  const current = () => {
    const [bot] = queryClient.getQueryData(api.keys.bots)
    return [api.botMood(bot), bot.preview]
  }
  const bot = { id: 'bot-1', status: 'idle', preview: 'Ready' }
  emit(sources[0], 'snapshot', { bots: [bot] })
  expect(current()).toEqual(['idle', 'Ready'])
  emit(sources[0], 'changed', { ...bot, status: 'running', thinking: true, preview: 'Starting' })
  expect(current()).toEqual(['thinking', 'Starting'])
  emit(sources[0], 'changed', { ...bot, status: 'running', preview: 'Using a tool' })
  expect(current()).toEqual(['working', 'Using a tool'])

  sources[0].onerror()
  expect(sources[0].closed).toBe(true)
  retry()
  await settle()
  expect(sources[1]?.url.href).toBe(`${origin}/v1/bots/events?ticket=read-ticket-2`)
  emit(sources[1], 'snapshot', { bots: [{ ...bot, preview: 'Finished' }] })
  expect(current()).toEqual(['idle', 'Finished'])

  api.clearApiSession()
  expect(sources[1].closed).toBe(true)
  emit(sources[1], 'changed', { ...bot, status: 'running' })
  expect(current()).toEqual(['idle', 'Finished'])
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

test('cookie-authenticated Cloud Web sessions attach credentials: include without overriding explicit omit', async () => {
  const origin = 'https://cloud-staging.example.com'
  const { api } = fixture(origin)
  api.setApiServerMode('cloud')
  api.setApiAuthToken(origin, null)
  const calls = []
  let rejections = 0
  const unsub = api.onApiAuthRejected(() => {
    rejections += 1
  })
  replace('fetch', async (url, init) => {
    calls.push({
      url: String(url),
      credentials: init.credentials,
      auth: init.headers.get('Authorization'),
    })
    if (String(url).endsWith('/v1/expired')) {
      return Response.json({ error: 'invalid session token', code: 'unauthorized' }, { status: 401 })
    }
    return Response.json({ ok: true })
  })

  await api.apiFetch('/v1/threads')
  await api.apiFetch('/v1/public', { credentials: 'omit' })
  await api.apiFetch('/v1/expired')
  unsub()

  expect(calls).toEqual([
    { url: `${origin}/v1/threads`, credentials: 'include', auth: null },
    { url: `${origin}/v1/public`, credentials: 'omit', auth: null },
    { url: `${origin}/v1/expired`, credentials: 'include', auth: null },
  ])
  expect(rejections).toBe(1)
})
