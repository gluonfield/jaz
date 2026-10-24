import { afterEach, expect, test } from 'bun:test'
import { build } from 'bun'
import { createRequire } from 'node:module'
import * as React from 'react'

const bundle = await build({
  entrypoints: [new URL('./connection.ts', import.meta.url).pathname],
  target: 'browser',
  format: 'cjs',
  external: ['react'],
  define: { 'import.meta.env': '{}' },
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

async function fixture({ auth, pairing = false, storedValues = [], decryptCredential } = {}) {
  const stored = new Map(storedValues)
  const timers = new Map()
  let nextTimer = 0
  replace('window', {
    location: { origin: 'http://localhost', search: '', hash: '' },
    addEventListener() {},
    jaz: {
      apiBaseUrl: 'http://127.0.0.1:5299',
      decryptCredential,
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
  replace('setTimeout', (callback, delay) => {
    timers.set(++nextTimer, { callback, delay })
    return nextTimer
  })
  replace('clearTimeout', (id) => timers.delete(id))
  replace('fetch', async (url) => {
    const target = new URL(url)
    if (target.hostname === '127.0.0.1') {
      return Response.json({ ok: false })
    }
    if (target.pathname === '/health') {
      return Response.json({ ok: true, auth_required: true })
    }
    if (target.pathname === '/v1/devices/register') {
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
    ? { ...React, useSyncExternalStore: (_subscribe, snapshot) => snapshot() }
    : require(name), module, module.exports)
  connection = module.exports
  await settle()
  return {
    connection,
    stored,
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
  expect(connection.useConnection().error).toBe('System keychain is locked')
  expect(stored.get('jaz.backendAuth.' + target)).toBe('enc:v1:sealed-credential')
})
