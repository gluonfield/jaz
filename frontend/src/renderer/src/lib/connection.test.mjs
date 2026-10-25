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
        contents: `export * from ${JSON.stringify(new URL('./connection.ts', import.meta.url).pathname)}\nexport { ThreadNotifications } from ${JSON.stringify(new URL('../components/notifications/ThreadNotifications.tsx', import.meta.url).pathname)}`,
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

async function fixture({ auth, pairing = false, storedValues = [], encryptCredential, decryptCredential, health, registration, notifications = false, startLocalBackend, renderNotifications = false } = {}) {
  const stored = new Map(storedValues)
  const configured = []
  const timers = new Map()
  let nextTimer = 0
  replace('window', {
    location: { origin: 'http://localhost', search: '', hash: '' },
    addEventListener() {},
    jaz: {
      apiBaseUrl: 'http://127.0.0.1:5299',
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
      return Response.json({ token: 'device-token' })
    }
    if (target.pathname === '/v1/devices/pairing-requests' && pairing) {
      return Response.json({ pairing: { id: 'pair-1', expires_at: new Date(Date.now() + 60_000).toISOString() }, pairing_secret: 'proof' }, { status: 202 })
    }
    if (target.pathname === '/v1/devices/pairing-requests/pair-1') {
      return Response.json({ pairing: { status: 'approved' }, token: 'device-token' })
    }
    if (target.pathname === '/v1/auth/check') {
      return auth()
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
    async pollPairing() {
      const timer = [...timers.values()].find((entry) => entry.delay === 2_000)
      await timer.callback()
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

test('local registration reports secure credential storage failure and recovers on an authorized retry', async () => {
  const target = 'http://127.0.0.1:5299'
  let denied = true
  let authChecks = 0
  const registrations = []
  const { connection, stored } = await fixture({
    storedValues: [['jaz.backendAuth.' + target, 'enc:v1:previous-ciphertext']],
    decryptCredential: () => 'expired-device-token',
    startLocalBackend: async () => ({ ok: true, url: target, key: 'local-bootstrap-key' }),
    registration(init) {
      registrations.push({ headers: init.headers, body: JSON.parse(init.body) })
      return Response.json({ token: 'new-device-token' })
    },
    encryptCredential(value) {
      expect(value).toBe('new-device-token')
      if (denied) {
        throw new Error('Error while encrypting text')
      }
      return 'new-ciphertext'
    },
    auth: () => {
      authChecks += 1
      return Response.json({ ok: true, auth_kind: 'device' })
    },
  })
  const error = await connection.startLocal()
  expect(error).toContain('Device registered')
  expect(error).toContain('system keychain')
  expect(connection.useConnection().status).toBe('disconnected')
  expect(stored.get('jaz.backendAuth.' + target)).toBe('enc:v1:previous-ciphertext')
  expect(authChecks).toBe(0)
  expect(registrations).toHaveLength(1)
  expect(registrations[0].headers.Authorization).toBe('Bearer local-bootstrap-key')
  expect(registrations[0].body).toEqual({ device_id: 'identity', public_key: 'public', name: 'Test device', kind: 'desktop' })
  denied = false
  expect(await connection.startLocal()).toBeNull()
  expect(connection.useConnection().status).toBe('connected')
  expect(authChecks).toBe(1)
  expect(registrations).toHaveLength(2)
  expect(registrations[1].body).toEqual(registrations[0].body)
  expect(stored.get('jaz.backendAuth.' + target)).toBe('enc:v1:new-ciphertext')
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
  respond(Response.json({ ok: true, auth_kind: 'device' }))
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
  expect(configured).toEqual([{ enabled: false }])
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
      return 'device-token'
    },
    auth: () => Response.json({ ok: true, auth_kind: 'device' }),
    notifications: true,
  }
  const first = await fixture(options)
  expect(events).toEqual(['health'])
  online = true
  const pending = [...first.timers.entries()].find(([, timer]) => timer.delay === 5_000)
  first.timers.delete(pending[0])
  await pending[1].callback()
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
  expect(restarted.configured[0]).toEqual({ enabled: true, baseUrl: target, token: 'device-token' })
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

test('an optional notification credential cannot tear down an unauthenticated connection', async () => {
  const target = 'https://jaz.example'
  const { connection, configured, stored } = await fixture({
    notifications: true,
    storedValues: [
      ['jaz.connection', JSON.stringify({ mode: 'remote', remoteUrl: target })],
      ['jaz.backendAuth.' + target, 'enc:v1:sealed-credential'],
    ],
    health: () => Response.json({ ok: true, auth_required: false }),
    decryptCredential() {
      throw new Error('Keychain denied')
    },
  })
  expect(() => connection.ThreadNotifications()).not.toThrow()
  expect(connection.useConnection().status).toBe('connected')
  expect(configured).toEqual([{ enabled: false }])
  expect(stored.get('jaz.backendAuth.' + target)).toBe('enc:v1:sealed-credential')
})
