import { afterEach, expect, test } from 'bun:test'
import { build } from 'bun'
import { createRequire } from 'node:module'
import * as React from 'react'

const bundle = await build({
  entrypoints: ['connection-fixture'],
  target: 'browser',
  format: 'cjs',
  external: ['react'],
  define: { 'import.meta.env': '{}' },
  plugins: [{
    name: 'connection-fixture',
    setup(builder) {
      builder.onResolve({ filter: /^connection-fixture$/ }, () => ({ path: 'entry', namespace: 'fixture' }))
      builder.onResolve({ filter: /^\//, namespace: 'fixture' }, (args) => ({ path: args.path, namespace: 'file' }))
      builder.onLoad({ filter: /.*/, namespace: 'fixture' }, () => ({
        contents: `export * from ${JSON.stringify(new URL('./connection.ts', import.meta.url).pathname)}\nexport * from ${JSON.stringify(new URL('./api/client.ts', import.meta.url).pathname)}\nexport { ThreadNotifications } from ${JSON.stringify(new URL('../components/notifications/ThreadNotifications.tsx', import.meta.url).pathname)}`,
        loader: 'ts',
      }))
    },
  }],
})
if (!bundle.success) {
  throw new AggregateError(bundle.logs)
}
const evaluate = new Function('require', 'module', 'exports', await bundle.outputs[0].text())
const require = createRequire(import.meta.url)
const originals = new Map()
let connection

function replace(name, value) {
  if (!originals.has(name)) {
    originals.set(name, Object.getOwnPropertyDescriptor(globalThis, name))
  }
  Object.defineProperty(globalThis, name, { configurable: true, writable: true, value })
}

afterEach(() => {
  connection?.disconnectBackend()
  connection = undefined
  for (const [name, descriptor] of originals) {
    if (descriptor) {
      Object.defineProperty(globalThis, name, descriptor)
    } else {
      delete globalThis[name]
    }
  }
  originals.clear()
})

async function settle() {
  for (let i = 0; i < 30; i += 1) {
    await Promise.resolve()
  }
}

async function fixture({ auth, pairing = false, storedValues = [], encryptCredential, decryptCredential, health, registration, notifications = false, startLocalBackend, renderNotifications = false, owner = true, initialSnapshot, feature } = {}) {
  const stored = new Map(storedValues)
  const configured = []
  const timers = new Map()
  let nextTimer = 0
  let snapshot = initialSnapshot || { revision: 0, phase: 'initial', url: '', session: null, error: null, localPending: false }
  const session = {
    read: () => ({ document: 1, owner, snapshot }),
    subscribe: () => () => {},
    update(request) {
      snapshot = request.operation === 'begin'
        ? { ...snapshot, revision: snapshot.revision + 1, phase: 'pending', url: request.url, session: null }
        : request.operation === 'commit'
          ? { ...snapshot, revision: snapshot.revision + 1, phase: 'connected', session: request.session }
          : { ...snapshot, revision: snapshot.revision + 1, phase: 'blocked', session: null, error: request.error }
      return snapshot
    },
    async local() {
      const result = await startLocalBackend?.()
      if (!result?.ok) {
        throw new Error(result?.error || 'Fixture local start unavailable')
      }
      return { url: result.url, token: 'jaz_local-device-token', deviceId: 'identity', remembered: false }
    },
  }
  replace('window', {
    location: { origin: 'http://localhost', search: '', hash: '' },
    addEventListener() {},
    jaz: {
      apiBaseUrl: 'http://127.0.0.1:5299',
      windowKind: owner ? 'main' : 'board',
      connectionSession: session,
      encryptCredential,
      decryptCredential,
      startLocalBackend,
      ...(notifications ? { configureThreadNotifications: (config) => configured.push(config) } : {}),
      getDeviceIdentity: async () => ({ device_id: 'identity', public_key: 'public' }),
      getDeviceMetadata: async () => ({ name: 'Test device' }),
    },
  })
  replace('document', { visibilityState: 'visible', addEventListener() {} })
  replace('localStorage', {
    getItem: (key) => stored.get(key) ?? null,
    setItem: (key, value) => stored.set(key, value),
    removeItem: (key) => stored.delete(key),
  })
  window.localStorage = localStorage
  replace('setTimeout', (callback, delay) => {
    timers.set(++nextTimer, { callback, delay })
    return nextTimer
  })
  replace('clearTimeout', (id) => timers.delete(id))
  replace('fetch', async (url, init) => {
    const target = new URL(url)
    if (target.hostname === '127.0.0.1' && target.pathname === '/health') {
      return Response.json({ ok: false })
    }
    if (target.pathname === '/health') {
      return health ? health() : Response.json({ ok: true, auth_required: true })
    }
    if (target.pathname === '/v1/devices/register') {
      if (registration) {
        return registration(init)
      }
      return Response.json({ token: 'jaz_device-token' })
    }
    if (target.pathname === '/v1/devices/pairing-requests' && pairing) {
      return Response.json({ pairing: { id: 'pair-1', expires_at: new Date(Date.now() + 60_000).toISOString() }, pairing_secret: 'proof' }, { status: 202 })
    }
    if (target.pathname === '/v1/devices/pairing-requests/pair-1') {
      return Response.json({ pairing: { status: 'approved' }, token: 'jaz_device-token' })
    }
    if (target.pathname === '/v1/auth/check') {
      return auth(init)
    }
    if (feature) {
      return feature(init)
    }
    throw new Error(`Unexpected request ${url}`)
  })
  const module = { exports: {} }
  evaluate((name) => name === 'react'
    ? { ...React, useSyncExternalStore: (_subscribe, snapshot) => snapshot(), useEffect: (effect) => effect() }
    : require(name), module, module.exports)
  connection = module.exports
  if (renderNotifications) {
    connection.ThreadNotifications()
  }
  await settle()
  return {
    connection,
    stored,
    configured,
    timers,
    session,
    async pollPairing() {
      const timer = [...timers.values()].find((entry) => entry.delay === 2_000)
      await timer.callback()
      await settle()
    },
  }
}

test('an enrollment credential rejected by verification does not connect', async () => {
  const { connection } = await fixture({ auth: () => Response.json({ error: 'revoked' }, { status: 401 }) })
  expect(await connection.connectRemote('https://jaz.example#key=invite_once')).toBe('Backend key was rejected')
  expect(connection.useConnection().status).toBe('disconnected')
})

test('a redeemed pairing credential rejected by verification does not connect', async () => {
  const { connection, pollPairing } = await fixture({ pairing: true, auth: () => Response.json({ error: 'revoked' }, { status: 401 }) })
  expect(await connection.connectRemote('https://jaz.example')).toBeNull()
  expect(connection.useConnection().status).toBe('pending_approval')
  await pollPairing()
  expect(connection.useConnection().status).toBe('disconnected')
})

test('local startup authenticates without touching encrypted storage', async () => {
  const target = 'http://127.0.0.1:5299'
  let unlocks = 0
  let encryptions = 0
  const { connection, stored } = await fixture({
    storedValues: [['jaz.backendAuth.' + target, 'enc:v1:previous-ciphertext']],
    decryptCredential() {
      unlocks += 1
      throw new Error('Denied')
    },
    encryptCredential() {
      encryptions += 1
      throw new Error('Denied')
    },
    startLocalBackend: async () => ({ ok: true, url: target, key: 'local-bootstrap-key' }),
  })
  expect(await connection.startLocal()).toBeNull()
  expect(connection.useConnection().status).toBe('connected')
  expect(connection.cachedApiAuthToken(target)).toBe('jaz_local-device-token')
  expect(stored.get('jaz.backendAuth.' + target)).toBe('enc:v1:previous-ciphertext')
  connection.disconnectBackend()
  expect(connection.cachedApiAuthToken(target)).toBe('')
  expect(stored.get('jaz.backendAuth.' + target)).toBe('enc:v1:previous-ciphertext')
  expect(unlocks).toBe(0)
  expect(encryptions).toBe(0)
})

test('registration rejection keeps the backend error and never saves a credential', async () => {
  let encryptions = 0
  const { connection } = await fixture({
    registration: () => Response.json({ error: 'device identity is invalid' }, { status: 400 }),
    encryptCredential() {
      encryptions += 1
      throw new Error('Should not encrypt')
    },
  })
  expect(await connection.connectRemote('https://jaz.example#key=invite_once')).toBe('device identity is invalid')
  expect(connection.useConnection().status).toBe('disconnected')
  expect(encryptions).toBe(0)
})

test('disconnecting while enrollment verification runs wins over its late response', async () => {
  let respond
  let requested
  const requestStarted = new Promise((resolve) => {
    requested = resolve
  })
  const { connection } = await fixture({ auth: () => {
    requested()
    return new Promise((resolve) => {
      respond = resolve
    })
  } })
  const attempt = connection.connectRemote('https://jaz.example#key=invite_once')
  await requestStarted
  connection.disconnectBackend()
  respond(Response.json({ ok: true, auth_kind: 'device', device_id: 'identity' }))
  await attempt
  expect(connection.useConnection().status).toBe('disconnected')
})


test('a locked credential store leaves startup in an actionable disconnected state', async () => {
  const target = 'https://jaz.example'
  const { connection, stored } = await fixture({
    storedValues: [
      ['jaz.backendUrl', target],
      ['jaz.connection', JSON.stringify({ mode: 'remote', remoteUrl: target })],
      ['jaz.backendAuth.' + target, 'enc:v1:sealed-credential'],
    ],
    decryptCredential() {
      throw new Error('System keychain is locked')
    },
  })
  expect(connection.useConnection().status).toBe('disconnected')
  expect(connection.useConnection().error).toContain('restart Jaz')
  expect(stored.get('jaz.backendAuth.' + target)).toBe('enc:v1:sealed-credential')
})

test.each([false, true])('offline startup never unlocks a credential for notifications (enabled: %j)', async (enabled) => {
  let unlocks = 0
  const { connection, configured } = await fixture({
    notifications: true,
    renderNotifications: true,
    storedValues: [
      ['jaz.backendUrl', 'https://jaz.example'],
      ['jaz.backendAuth.https://jaz.example', 'enc:v1:sealed-credential'],
      ['jaz.notifications.threadFinished', String(enabled)],
    ],
    decryptCredential() {
      unlocks += 1
      throw new Error('Keychain denied')
    },
  })
  expect(connection.useConnection().status).toBe('disconnected')
  expect(unlocks).toBe(0)
  expect(configured.every((value) => value.enabled === false)).toBe(true)
})

test('remote startup waits for health before unlocking, stops on denial, and recovers after restart', async () => {
  const target = 'https://jaz.example'
  let online = false
  let denied = true
  const events = []
  const storedValues = [
    ['jaz.backendUrl', target],
    ['jaz.connection', JSON.stringify({ mode: 'remote', remoteUrl: target })],
    ['jaz.backendAuth.' + target, 'enc:v1:sealed-credential'],
  ]
  const options = {
    storedValues,
    health: () => {
      events.push('health')
      return Response.json({ ok: online, auth_required: true }, { status: online ? 200 : 503 })
    },
    decryptCredential() {
      events.push('unlock')
      if (denied) {
        throw new Error('Keychain denied')
      }
      return 'jaz_device-token'
    },
    auth: () => Response.json({ ok: true, auth_kind: 'device', device_id: 'identity' }),
    notifications: true,
  }
  const first = await fixture(options)
  expect(events).toEqual(['health'])
  online = true
  const pending = [...first.timers.entries()].find(([, timer]) => timer.delay === 5_000)
  first.timers.delete(pending[0])
  await pending[1].callback()
  await settle()
  first.connection.ThreadNotifications()
  expect(events).toEqual(['health', 'health', 'unlock'])
  expect(first.connection.useConnection().error).toContain('restart Jaz')
  expect(first.timers.size).toBe(0)
  expect(first.stored.get('jaz.backendAuth.' + target)).toBe('enc:v1:sealed-credential')
  first.connection.disconnectBackend()
  denied = false
  const restarted = await fixture(options)
  expect(restarted.connection.useConnection().status).toBe('connected')
  restarted.connection.ThreadNotifications()
  expect(events).toEqual(['health', 'health', 'unlock', 'health', 'unlock'])
  expect(restarted.configured).toContainEqual({ enabled: true, baseUrl: target, token: 'jaz_device-token' })
  expect(restarted.stored.get('jaz.backendAuth.' + target)).toBe('enc:v1:sealed-credential')
})

test('an unauthenticated health probe does not unlock saved credentials', async () => {
  let unlocks = 0
  const target = 'https://jaz.example'
  const { connection } = await fixture({
    storedValues: [
      ['jaz.connection', JSON.stringify({ mode: 'remote', remoteUrl: target })],
      ['jaz.backendAuth.' + target, 'enc:v1:sealed-credential'],
    ],
    health: () => Response.json({ ok: true, auth_required: false }),
    decryptCredential() {
      unlocks += 1
      throw new Error('Keychain denied')
    },
  })
  expect(connection.useConnection().status).toBe('connected')
  expect(await connection.connectRemote(target)).toBeNull()
  expect(unlocks).toBe(0)
})

test('a failed local launch reports its error before touching saved credentials', async () => {
  let unlocks = 0
  const { connection } = await fixture({
    storedValues: [['jaz.backendAuth.http://127.0.0.1:5299', 'enc:v1:sealed-credential']],
    startLocalBackend: async () => ({ ok: false, error: 'Fixture backend could not start' }),
    decryptCredential() {
      unlocks += 1
      throw new Error('Keychain denied')
    },
  })
  expect(await connection.startLocal()).toBe('Fixture backend could not start')
  expect(unlocks).toBe(0)
})

test.each([false, true])('notifications use only credentials resolved by connection verification (auth_required: %j)', async (authRequired) => {
  const target = 'https://jaz.example'
  let unlocks = 0
  let authChecks = 0
  const { connection, configured, stored } = await fixture({
    notifications: true,
    storedValues: [
      ['jaz.connection', JSON.stringify({ mode: 'remote', remoteUrl: target })],
      ['jaz.backendAuth.' + target, 'enc:v1:sealed-credential'],
    ],
    health: () => Response.json({ ok: true, auth_required: authRequired }),
    decryptCredential() {
      unlocks += 1
      return 'jaz_device-token'
    },
    auth() {
      authChecks += 1
      return Response.json({ ok: true, auth_kind: 'device', device_id: 'identity' })
    },
  })
  expect(connection.useConnection().status).toBe('connected')
  expect(unlocks).toBe(authRequired ? 1 : 0)
  connection.ThreadNotifications()
  expect(unlocks).toBe(authRequired ? 1 : 0)
  expect(authChecks).toBe(authRequired ? 1 : 0)
  expect(configured).toEqual([{ enabled: true, baseUrl: target, token: authRequired ? 'jaz_device-token' : '' }])

  expect(stored.get('jaz.backendAuth.' + target)).toBe('enc:v1:sealed-credential')
})

test('a late same-origin candidate cannot replace the verified token or ticket', async () => {
  let respond
  let requested
  const waiting = new Promise((resolve) => {
    requested = resolve
  })
  const { connection, stored } = await fixture({ auth: (init) => {
    if (init.headers.Authorization === 'Bearer jaz_first') {
      requested()
      return new Promise((resolve) => {
        respond = resolve
      })
    }
    return Response.json({ ok: true, auth_kind: 'device', device_id: 'identity', resource_ticket: { token: 'new-ticket', expires_at: '2099-01-01T00:00:00Z' } })
  } })
  const first = connection.connectRemote('https://jaz.example#key=jaz_first')
  await waiting
  expect(await connection.connectRemote('https://jaz.example#key=jaz_second')).toBeNull()
  respond(Response.json({ ok: true, auth_kind: 'device', device_id: 'identity', resource_ticket: { token: 'old-ticket', expires_at: '2099-01-01T00:00:00Z' } }))
  await first
  expect(connection.cachedApiAuthToken('https://jaz.example')).toBe('jaz_second')
  expect(connection.apiEmbeddedGetUrl('/v1/sessions/thread/file?raw=1')).toContain('new-ticket')
  expect(connection.connectionPreference()).toBeNull()
  expect(stored.has('jaz.backendAuth.https://jaz.example')).toBe(false)
})

test('explicit remember failure preserves the verified session and prior ciphertext', async () => {
  const target = 'https://jaz.example'
  const { connection, stored } = await fixture({
    storedValues: [['jaz.backendAuth.' + target, 'enc:v1:older']],
    encryptCredential() {
      throw new Error('Denied')
    },
    decryptCredential() {
      throw new Error('Must not unlock')
    },
    auth: () => Response.json({ ok: true, auth_kind: 'device', device_id: 'identity' }),
  })
  expect(await connection.connectRemote(target + '#key=jaz_fresh')).toBeNull()
  expect(connection.rememberConnection()).toContain('Connected for this session')
  expect(connection.cachedApiAuthToken(target)).toBe('jaz_fresh')
  expect(connection.useConnection().status).toBe('connected')
  expect(stored.get('jaz.backendAuth.' + target)).toBe('enc:v1:older')
  expect(connection.connectionPreference()).toBeNull()
})

test.each([
  { ok: true, auth_kind: 'root' },
  { ok: true, auth_kind: 'device', device_id: 'wrong' },
  { auth_kind: 'device', device_id: 'identity' },
])('issued credentials require the expected verified device: %j', async (body) => {
  const { connection } = await fixture({ auth: () => Response.json(body) })
  expect(await connection.connectRemote('https://jaz.example#key=invite_once')).toContain('expected device')
  expect(connection.cachedApiAuthToken('https://jaz.example')).toBe('')
  expect(connection.useConnection().status).toBe('disconnected')
})

test('an old URL alone never restores encrypted remote credentials', async () => {
  let unlocks = 0
  const { connection } = await fixture({
    storedValues: [['jaz.backendUrl', 'https://jaz.example'], ['jaz.backendAuth.https://jaz.example', 'enc:v1:older']],
    decryptCredential() {
      unlocks += 1
    },
  })
  expect(connection.useConnection().status).toBe('disconnected')
  expect(unlocks).toBe(0)
})


test('feature denial and a separate rejected root grant preserve the device session; auth rejection clears it', async () => {
  let rejected = false
  const { connection } = await fixture({
    auth: () => Response.json({ ok: true, auth_kind: 'device', device_id: 'identity', resource_ticket: { token: 'read-ticket', expires_at: '2099-01-01T00:00:00Z' } }),
    feature: () => rejected
      ? Response.json({ error: 'revoked', code: 'unauthorized' }, { status: 401 })
      : Response.json({ error: 'Computer Use is disabled' }, { status: 403 }),
  })
  await connection.connectRemote('https://jaz.example#key=jaz_verified')
  const forbidden = await connection.apiFetch('/fixture/feature')
  expect(forbidden.status).toBe(403)
  expect(await forbidden.json()).toEqual({ error: 'Computer Use is disabled' })
  expect(connection.useConnection().status).toBe('connected')
  expect(connection.cachedApiAuthToken()).toBe('jaz_verified')
  expect(connection.apiEmbeddedGetUrl('/fixture/file')).toContain('read-ticket')
  rejected = true
  const hostDenied = await connection.apiFetch('/v1/devices', { headers: { Authorization: 'Bearer separate-root' } })
  expect(hostDenied.status).toBe(401)
  await hostDenied.json()
  expect(connection.useConnection().status).toBe('connected')
  await connection.apiFetch('/fixture/feature').catch(() => null)
  expect(connection.useConnection().status).toBe('disconnected')
  expect(connection.cachedApiAuthToken()).toBe('')
  expect(connection.apiEmbeddedGetUrl('/fixture/file')).not.toContain('ticket=')
})

test.each([true, false])('recoverable hydration retains the snapshot and retries without credentials work (owner: %j)', async (owner) => {
  let checks = 0
  let keychain = 0
  const url = 'https://jaz.example'
  const snapshot = { revision: 4, phase: 'connected', url, session: { url, token: 'jaz_verified', deviceId: 'identity', remembered: false }, error: null, localPending: false }
  const { connection, session, timers } = await fixture({
    owner, initialSnapshot: snapshot,
    encryptCredential() {
      keychain += 1
    },
    decryptCredential() {
      keychain += 1
    },
    registration() {
      throw new Error('Must not register')
    },
    auth: () => ++checks === 1
      ? Response.json({ error: 'temporary' }, { status: 503 })
      : Response.json({ ok: true, auth_kind: 'device', device_id: 'identity' }),
  })
  expect(connection.useConnection().status).toBe('disconnected')
  expect(session.read().snapshot).toBe(snapshot)
  expect(connection.cachedApiAuthToken(url)).toBe('')
  await [...timers.values()].find((entry) => entry.delay === 5000).callback()
  await settle()
  expect(connection.useConnection().status).toBe('connected')
  expect(connection.cachedApiAuthToken(url)).toBe('jaz_verified')
  expect(checks).toBe(2)
  expect(keychain).toBe(0)
})

test('reusing a remembered current token keeps restart provenance without unlocking or saving again', async () => {
  const url = 'https://jaz.example'
  let saves = 0
  let unlocks = 0
  const { connection, stored } = await fixture({
    encryptCredential(token) {
      saves += 1;
      return 'sealed-' + token
    },
    decryptCredential() {
      unlocks += 1;
      throw new Error('Must not unlock')
    },
    auth: () => Response.json({ ok: true, auth_kind: 'device', device_id: 'identity' }),
  })
  await connection.connectRemote(url + '#key=jaz_first')
  expect(connection.rememberConnection()).toBeNull()
  expect(await connection.connectRemote(url)).toBeNull()
  expect(connection.connectionPreference()).toEqual({ mode: 'remote', remoteUrl: url })
  expect(saves).toBe(1)
  expect(unlocks).toBe(0)
  expect(await connection.connectRemote(url + '#key=jaz_fresh')).toBeNull()
  expect(connection.connectionPreference()).toBeNull()
  expect(stored.get('jaz.backendAuth.' + url)).toBe('enc:v1:sealed-jaz_first')
})
