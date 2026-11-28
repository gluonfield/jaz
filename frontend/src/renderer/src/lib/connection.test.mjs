import { afterEach, expect, test } from 'bun:test'
import { build } from 'bun'
import { createRequire } from 'node:module'
import * as React from 'react'

const bundle = await build({
  entrypoints: ['connection-fixture'],
  target: 'browser',
  format: 'cjs',
  external: ['react', '@/lib/browserCredentials'],
  define: { 'import.meta.env': '{}' },
  plugins: [{
    name: 'connection-fixture',
    setup(builder) {
      builder.onResolve({ filter: /^connection-fixture$/ }, () => ({ path: 'entry', namespace: 'fixture' }))
      builder.onResolve({ filter: /^\//, namespace: 'fixture' }, (args) => ({ path: args.path, namespace: 'file' }))
      builder.onLoad({ filter: /.*/, namespace: 'fixture' }, () => ({
        contents: `export * from ${JSON.stringify(new URL('./connection.ts', import.meta.url).pathname)}\nexport * from ${JSON.stringify(new URL('./api/client.ts', import.meta.url).pathname)}\nexport * from ${JSON.stringify(new URL('./api/cloudAuth.ts', import.meta.url).pathname)}\nexport { DesktopNotifications } from ${JSON.stringify(new URL('../components/notifications/DesktopNotifications.tsx', import.meta.url).pathname)}`,
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

async function fixture({ auth, authMe, authExchange, authRefresh, authLogout, cloudConfig, discovery, storedValues = [], sessionStoredValues = [], encryptCredential, decryptCredential, health, notifications = false, startLocalBackend, renderNotifications = false, owner = true, attached = true, windowKind = owner ? 'main' : 'board', initialSnapshot, feature, web = false, locationHref = 'http://localhost/', browserCredentials = new Map(), saveBrowserCredential, loadBrowserCredential, cloudUrl, localEnabled } = {}) {
  const stored = new Map(storedValues)
  const sessionStored = new Map(sessionStoredValues)
  const configured = []
  const openedExternal = []
  let oidcBeginCalls = 0
  let oidcCallbackHandler = null
  const windowListeners = new Map()
  const timers = new Map()
  let nextTimer = 0
  const loc = new URL(locationHref)
  let snapshot = initialSnapshot || { revision: 0, phase: 'initial', url: '', session: null, error: null, localPending: false }
  const session = {
    read: () => attached ? { document: 1, owner, snapshot } : null,
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
      return { url: result.url, token: 'local-bearer-token', remembered: false }
    },
  }
  replace('window', {
    location: {
      href: loc.href,
      origin: loc.origin,
      pathname: loc.pathname,
      search: loc.search,
      hash: loc.hash,
      assign(target) {
        openedExternal.push(String(target))
      },
    },
    history: {
      state: null,
      replaceState(_state, _title, next) {
        const updated = new URL(String(next))
        window.location.href = updated.href
        window.location.search = updated.search
        window.location.hash = updated.hash
      },
    },
    addEventListener(name, handler) {
      const list = windowListeners.get(name) ?? []
      list.push(handler)
      windowListeners.set(name, list)
    },
    open(target) {
      openedExternal.push(String(target))
    },
    jaz: web ? undefined : {
      apiBaseUrl: 'http://127.0.0.1:5299',
      cloudUrl,
      localEnabled,
      windowKind,
      connectionSession: session,
      encryptCredential,
      decryptCredential,
      startLocalBackend,
      beginOidcSignIn: async () => {
        oidcBeginCalls += 1
        return { redirectUri: 'http://127.0.0.1:43123/auth/callback' }
      },
      onOidcCallback: (handler) => {
        oidcCallbackHandler = handler
        return () => {
          if (oidcCallbackHandler === handler) oidcCallbackHandler = null
        }
      },
      openExternalURL: (target) => openedExternal.push(String(target)),
      ...(notifications ? { configureNotifications: (config) => configured.push(config) } : {}),
    },
  })
  replace('document', { visibilityState: 'visible', addEventListener() {} })
  replace('localStorage', {
    getItem: (key) => stored.get(key) ?? null,
    setItem: (key, value) => stored.set(key, value),
    removeItem: (key) => stored.delete(key),
  })
  replace('sessionStorage', {
    getItem: (key) => sessionStored.get(key) ?? null,
    setItem: (key, value) => sessionStored.set(key, value),
    removeItem: (key) => sessionStored.delete(key),
  })
  window.localStorage = localStorage
  window.sessionStorage = sessionStorage
  replace('setTimeout', (callback, delay) => {
    timers.set(++nextTimer, { callback, delay })
    return nextTimer
  })
  replace('clearTimeout', (id) => timers.delete(id))
  replace('fetch', async (url, init) => {
    const target = new URL(url)
    if (target.pathname === '/cloud-config.json' && cloudConfig) {
      return cloudConfig(target, init)
    }
    if (target.pathname === '/.well-known/jaz-server' && discovery) {
      return discovery(target)
    }
    if (target.hostname === '127.0.0.1' && target.pathname === '/health') {
      return Response.json({ ok: false })
    }
    if (target.pathname === '/health') {
      return health ? health() : Response.json({ ok: true, auth_required: true })
    }
    if (target.pathname === '/v1/auth/exchange' && authExchange) {
      return authExchange(init)
    }
    if (target.pathname === '/v1/auth/refresh') {
      return authRefresh
        ? authRefresh(init)
        : Response.json({ error: 'refresh_token is required', code: 'unauthorized' }, { status: 401 })
    }
    if (target.pathname === '/v1/auth/me' && authMe) {
      return authMe(init)
    }
    if (target.pathname === '/v1/auth/logout') {
      return authLogout ? authLogout(init) : new Response(null, { status: 204 })
    }
    if (target.pathname === '/v1/auth/check') {
      return auth(init)
    }
    if (feature) {
      return feature(init, target)
    }
    throw new Error(`Unexpected request ${url}`)
  })
  const module = { exports: {} }
  evaluate((name) => name === 'react'
    ? { ...React, useSyncExternalStore: (_subscribe, snapshot) => snapshot(), useEffect: (effect) => effect() }
    : name === '@/lib/browserCredentials' ? {
      loadBrowserCredential: loadBrowserCredential || (async (url) => browserCredentials.get(url)),
      saveBrowserCredential: saveBrowserCredential || (async (url, token, current, refreshToken) => {
        if (!current()) throw new Error('Connection changed')
        browserCredentials.set(url, refreshToken ? { token, refreshToken } : { token })
      }),
      forgetBrowserCredential: async (url) => browserCredentials.delete(url),
    }
    : require(name), module, module.exports)
  connection = module.exports
  if (renderNotifications) {
    connection.DesktopNotifications()
  }
  await settle()
  return {
    connection,
    stored,
    sessionStored,
    configured,
    openedExternal,
    oidcBeginCalls: () => oidcBeginCalls,
    triggerElectronOidcCallback: (url) => oidcCallbackHandler?.(url),
    triggerHashChange: (hash) => {
      const updated = new URL(window.location.href)
      updated.hash = hash
      window.location.href = updated.href
      window.location.hash = updated.hash
      for (const fn of windowListeners.get('hashchange') ?? []) fn()
    },
    timers,
    session,
    browserCredentials,
  }
}

test.each([false, true])('a bearer credential rejected by verification does not connect (web: %j)', async (web) => {
  const { connection, browserCredentials } = await fixture({ web, auth: () => Response.json({ error: 'revoked' }, { status: 401 }) })
  expect(await connection.connectRemote('https://jaz.example#key=bad-key')).toBe('Backend key was rejected')
  expect(connection.useConnection().status).toBe('disconnected')
  expect(browserCredentials.size).toBe(0)
})

test('connecting to an authenticated remote server without a credential fails cleanly without pairing', async () => {
  const { connection } = await fixture()
  expect(await connection.connectRemote('https://jaz.example')).toContain('Authentication required')
  expect(connection.useConnection().status).toBe('disconnected')
})

test('desktop startup auto-starts local backend without touching encrypted storage', async () => {
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
  expect(connection.useConnection().status).toBe('connected')
  expect(connection.cachedApiAuthToken(target)).toBe('local-bearer-token')
  expect(stored.get('jaz.backendAuth.' + target)).toBe('enc:v1:previous-ciphertext')
  connection.disconnectBackend()
  expect(connection.cachedApiAuthToken(target)).toBe('')
  expect(stored.get('jaz.backendAuth.' + target)).toBe('enc:v1:previous-ciphertext')
  expect(await connection.startLocal()).toBeNull()
  expect(connection.useConnection().status).toBe('connected')
  expect(unlocks).toBe(0)
  expect(encryptions).toBe(0)
})

test('disconnecting while auth verification runs wins over its late response', async () => {
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
  const attempt = connection.connectRemote('https://jaz.example#key=bearer-key')
  await requestStarted
  connection.disconnectBackend()
  respond(Response.json({ ok: true, auth_kind: 'bearer' }))
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

test.each(['off', 'notch'])('offline startup never unlocks a credential for notifications (mode: %s)', async (mode) => {
  let unlocks = 0
  const { connection, configured } = await fixture({
    notifications: true,
    renderNotifications: true,
    storedValues: [
      ['jaz.backendUrl', 'https://jaz.example'],
      ['jaz.backendAuth.https://jaz.example', 'enc:v1:sealed-credential'],
      ['jaz.notifications.mode', mode],
    ],
    decryptCredential() {
      unlocks += 1
      throw new Error('Keychain denied')
    },
  })
  expect(connection.useConnection().status).toBe('disconnected')
  expect(unlocks).toBe(0)
  expect(configured.every((value) => value.mode === 'off')).toBe(true)
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
      return 'bearer-token'
    },
    auth: () => Response.json({ ok: true, auth_kind: 'bearer' }),
    notifications: true,
  }
  const first = await fixture(options)
  expect(events).toEqual(['health'])
  online = true
  const pending = [...first.timers.entries()].find(([, timer]) => timer.delay === 5_000)
  first.timers.delete(pending[0])
  await pending[1].callback()
  await settle()
  first.connection.DesktopNotifications()
  expect(events).toEqual(['health', 'health', 'unlock'])
  expect(first.connection.useConnection().error).toContain('restart Jaz')
  expect(first.timers.size).toBe(0)
  expect(first.stored.get('jaz.backendAuth.' + target)).toBe('enc:v1:sealed-credential')
  first.connection.disconnectBackend()
  denied = false
  const restarted = await fixture(options)
  expect(restarted.connection.useConnection().status).toBe('connected')
  restarted.connection.DesktopNotifications()
  expect(events).toEqual(['health', 'health', 'unlock', 'health', 'unlock'])
  expect(restarted.configured).toContainEqual({ mode: 'center', baseUrl: target, token: 'bearer-token' })
  expect(restarted.stored.get('jaz.backendAuth.' + target)).toBe('enc:v1:sealed-credential')
})

test('one missed health poll keeps the session; consecutive misses show reconnecting until the backend answers', async () => {
  const target = 'https://jaz.example'
  let online = true
  const { connection, timers } = await fixture({
    storedValues: [
      ['jaz.connection', JSON.stringify({ mode: 'remote', remoteUrl: target })],
      ['jaz.backendAuth.' + target, 'enc:v1:sealed-credential'],
    ],
    health: () => Response.json({ ok: online, auth_required: true }, { status: online ? 200 : 503 }),
    decryptCredential: () => 'bearer-token',
    auth: () => Response.json({ ok: true, auth_kind: 'bearer' }),
  })
  const poll = async (up) => {
    online = up
    const [id, timer] = [...timers.entries()].find(([, entry]) => entry.delay === 5_000)
    timers.delete(id)
    await timer.callback()
    await settle()
    return connection.useConnection().status
  }
  expect(connection.useConnection().status).toBe('connected')
  expect(await poll(false)).toBe('connected')
  expect(await poll(true)).toBe('connected')
  expect(await poll(false)).toBe('connected')
  expect(await poll(false)).toBe('reconnecting')
  expect(await poll(false)).toBe('reconnecting')
  expect(await poll(true)).toBe('connected')
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

test.each([false, true])('a failed remembered local startup is retried on the next launch (renderer reloaded: %j)', async (reloaded) => {
  const target = 'http://127.0.0.1:5299'
  const error = 'Backend startup timed out'
  const failed = await fixture({
    storedValues: [['jaz.connection', JSON.stringify({ mode: 'local' })]],
    initialSnapshot: reloaded
      ? { revision: 2, phase: 'blocked', url: target, session: null, error, localPending: false }
      : undefined,
    startLocalBackend: async () => ({ ok: false, error }),
  })
  expect(failed.connection.useConnection()).toMatchObject({ status: 'disconnected', error })

  const { connection } = await fixture({
    storedValues: [...failed.stored],
    startLocalBackend: async () => ({ ok: true, url: target }),
  })
  expect(connection.useConnection()).toMatchObject({ status: 'connected', url: target })
  connection.disconnectBackend()
  expect(connection.connectionPreference()).toBeNull()
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
      return 'bearer-token'
    },
    auth() {
      authChecks += 1
      return Response.json({ ok: true, auth_kind: 'bearer' })
    },
  })
  expect(connection.useConnection().status).toBe('connected')
  expect(unlocks).toBe(authRequired ? 1 : 0)
  connection.DesktopNotifications()
  expect(unlocks).toBe(authRequired ? 1 : 0)
  expect(authChecks).toBe(authRequired ? 1 : 0)
  expect(configured).toEqual([{ mode: 'center', baseUrl: target, token: authRequired ? 'bearer-token' : '' }])

  expect(stored.get('jaz.backendAuth.' + target)).toBe('enc:v1:sealed-credential')
})

test('a late same-origin candidate cannot replace the verified token or ticket', async () => {
  let respond
  let requested
  const waiting = new Promise((resolve) => {
    requested = resolve
  })
  const { connection, stored } = await fixture({ auth: (init) => {
    if (init.headers.Authorization === 'Bearer first-token') {
      requested()
      return new Promise((resolve) => {
        respond = resolve
      })
    }
    return Response.json({ ok: true, auth_kind: 'bearer', resource_ticket: { token: 'new-ticket', expires_at: '2099-01-01T00:00:00Z' } })
  } })
  const first = connection.connectRemote('https://jaz.example#key=first-token')
  await waiting
  expect(await connection.connectRemote('https://jaz.example#key=second-token')).toBeNull()
  respond(Response.json({ ok: true, auth_kind: 'bearer', resource_ticket: { token: 'old-ticket', expires_at: '2099-01-01T00:00:00Z' } }))
  await first
  expect(connection.cachedApiAuthToken('https://jaz.example')).toBe('second-token')
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
    auth: () => Response.json({ ok: true, auth_kind: 'bearer' }),
  })
  expect(await connection.connectRemote(target + '#key=fresh-token')).toBeNull()
  expect(connection.rememberConnection()).toContain('Connected for this session')
  expect(connection.cachedApiAuthToken(target)).toBe('fresh-token')
  expect(connection.useConnection().status).toBe('connected')
  expect(stored.get('jaz.backendAuth.' + target)).toBe('enc:v1:older')
  expect(connection.connectionPreference()).toBeNull()
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

test('feature 403 preserves the session; unauthorized 401 clears it', async () => {
  let rejected = false
  const { connection } = await fixture({
    auth: () => Response.json({ ok: true, auth_kind: 'bearer', resource_ticket: { token: 'read-ticket', expires_at: '2099-01-01T00:00:00Z' } }),
    feature: () => rejected
      ? Response.json({ error: 'revoked', code: 'unauthorized' }, { status: 401 })
      : Response.json({ error: 'Computer Use is disabled' }, { status: 403 }),
  })
  await connection.connectRemote('https://jaz.example#key=verified-token')
  const forbidden = await connection.apiFetch('/fixture/feature')
  expect(forbidden.status).toBe(403)
  expect(await forbidden.json()).toEqual({ error: 'Computer Use is disabled' })
  expect(connection.useConnection().status).toBe('connected')
  expect(connection.cachedApiAuthToken()).toBe('verified-token')
  expect(connection.apiEmbeddedGetUrl('/fixture/file')).toContain('read-ticket')
  rejected = true
  await connection.apiFetch('/fixture/feature').catch(() => null)
  expect(connection.useConnection().status).toBe('disconnected')
  expect(connection.cachedApiAuthToken()).toBe('')
  expect(connection.apiEmbeddedGetUrl('/fixture/file')).not.toContain('ticket=')
})

test.each([true, false])('recoverable hydration retains the snapshot and retries without credentials work (owner: %j)', async (owner) => {
  let checks = 0
  let keychain = 0
  const url = 'https://jaz.example'
  const snapshot = { revision: 4, phase: 'connected', url, session: { url, token: 'verified-token', remembered: false }, error: null, localPending: false }
  const { connection, session, timers } = await fixture({
    owner, initialSnapshot: snapshot,
    encryptCredential() {
      keychain += 1
    },
    decryptCredential() {
      keychain += 1
    },
    auth: () => ++checks === 1
      ? Response.json({ error: 'temporary' }, { status: 503 })
      : Response.json({ ok: true, auth_kind: 'bearer' }),
  })
  expect(connection.useConnection().status).toBe('disconnected')
  expect(session.read().snapshot).toBe(snapshot)
  expect(connection.cachedApiAuthToken(url)).toBe('')
  await [...timers.values()].find((entry) => entry.delay === 5000).callback()
  await settle()
  expect(connection.useConnection().status).toBe('connected')
  expect(connection.cachedApiAuthToken(url)).toBe('verified-token')
  expect(checks).toBe(2)
  expect(keychain).toBe(0)
})

test.each([
  ['an app window the main process attached', true, 'connected', 'verified-token'],
  ['an overlay window it did not attach', false, 'checking', ''],
])('%s shares the main window connection only when attached', async (_name, attached, status, token) => {
  const url = 'https://jaz.example'
  const snapshot = { revision: 2, phase: 'connected', url, session: { url, token: 'verified-token', remembered: false }, error: null, localPending: false }
  const { connection } = await fixture({
    owner: false,
    attached,
    windowKind: attached ? 'app' : 'voice',
    initialSnapshot: snapshot,
    auth: () => Response.json({ ok: true, auth_kind: 'bearer' }),
  })
  expect(connection.useConnection().status).toBe(status)
  expect(connection.cachedApiAuthToken(url) ?? '').toBe(token)
})

test('reusing a remembered current token keeps restart provenance without unlocking or saving again', async () => {
  const url = 'https://jaz.example'
  let saves = 0
  let unlocks = 0
  const { connection, stored } = await fixture({
    encryptCredential(token) {
      saves += 1
      return 'sealed-' + token
    },
    decryptCredential() {
      unlocks += 1
      throw new Error('Must not unlock')
    },
    auth: () => Response.json({ ok: true, auth_kind: 'bearer' }),
  })
  await connection.connectRemote(url + '#key=first-token')
  expect(connection.rememberConnection()).toBeNull()
  expect(await connection.connectRemote(url)).toBeNull()
  expect(connection.connectionPreference()).toEqual({ mode: 'remote', remoteUrl: url })
  expect(saves).toBe(1)
  expect(unlocks).toBe(0)
  expect(await connection.connectRemote(url + '#key=fresh-token')).toBeNull()
  expect(connection.connectionPreference()).toBeNull()
  expect(stored.get('jaz.backendAuth.' + url)).toBe('enc:v1:sealed-first-token')
})

test('browser bearer session survives a fresh renderer and clears on forget', async () => {
  const url = 'https://jaz.example'
  const auth = () => Response.json({ ok: true, auth_kind: 'bearer' })
  const first = await fixture({ web: true, auth })
  expect(await first.connection.connectRemote(url + '#key=bearer-token')).toBeNull()
  expect(first.connection.useConnection().status).toBe('connected')
  expect(first.browserCredentials.get(url)).toEqual({ token: 'bearer-token' })
  expect(first.stored.has('jaz.backendAuth.' + url)).toBe(false)
  const storedValues = [...first.stored]
  first.connection.disconnectBackend()
  let checks = 0
  const second = await fixture({
    web: true, storedValues, browserCredentials: first.browserCredentials,
    auth(init) {
      checks += 1
      expect(init.headers.Authorization).toBe('Bearer bearer-token')
      return auth()
    },
  })
  expect(second.connection.useConnection().status).toBe('connected')
  expect(checks).toBe(1)
  second.connection.forgetBackend(url + '/')
  await settle()
  expect(second.browserCredentials.has(url)).toBe(false)
  expect(second.connection.connectionPreference()).toBeNull()
})

test('browser storage failure preserves the verified session without saving launch authority', async () => {
  const url = 'https://jaz.example'
  const { connection, browserCredentials } = await fixture({
    web: true,
    saveBrowserCredential: async () => { throw new Error('Storage denied') },
    auth: () => Response.json({ ok: true, auth_kind: 'bearer' }),
  })
  expect(await connection.connectRemote(url + '#key=verified-token')).toBeNull()
  expect(connection.useConnection().status).toBe('connected')
  expect(connection.useConnection().error).toContain('Could not save the login in this browser')
  expect(connection.cachedApiAuthToken(url)).toBe('verified-token')
  expect(connection.connectionPreference()).toBeNull()
  expect(browserCredentials.size).toBe(0)
})

test('browser restore rejects invalid auth before publishing credentials', async () => {
  const url = 'https://jaz.example'
  const { connection } = await fixture({
    web: true,
    auth: () => Response.json({ error: 'Unauthorized', code: 'unauthorized' }, { status: 401 }),
    browserCredentials: new Map([[url, { token: 'saved-token' }]]),
    storedValues: [['jaz.connection', JSON.stringify({ mode: 'remote', remoteUrl: url })]],
  })
  expect(connection.useConnection().status).toBe('disconnected')
  expect(connection.cachedApiAuthToken(url)).toBe('')
  expect(connection.connectionPreference()).toBeNull()
})

test('browser credential cannot persist after a newer connection replaces it', async () => {
  const url = 'https://jaz.example'
  const saved = new Map()
  let release
  let saving
  const started = new Promise((resolve) => { saving = resolve })
  const waiting = new Promise((resolve) => { release = resolve })
  const { connection } = await fixture({
    web: true,
    async saveBrowserCredential(url, token, current) {
      if (token === 'first-token') {
        saving()
        await waiting
      }
      if (!current()) throw new Error('Connection changed')
      saved.set(url, { token })
    },
    auth: () => Response.json({ ok: true, auth_kind: 'bearer' }),
  })
  const first = connection.connectRemote(url + '#key=first-token')
  await started
  expect(await connection.connectRemote(url + '#key=second-token')).toBeNull()
  release()
  await first
  expect(saved.get(url)).toEqual({ token: 'second-token' })
  expect(connection.cachedApiAuthToken(url)).toBe('second-token')
})

test('a late browser storage rejection cannot disconnect a newer verified connection', async () => {
  const url = 'https://jaz.example'
  let rejectLoad
  const pending = new Promise((_resolve, reject) => { rejectLoad = reject })
  const { connection } = await fixture({
    web: true,
    storedValues: [['jaz.connection', JSON.stringify({ mode: 'remote', remoteUrl: url })]],
    loadBrowserCredential: () => pending,
    auth: () => Response.json({ ok: true, auth_kind: 'bearer' }),
  })
  expect(await connection.connectRemote('https://new.example#key=new-token')).toBeNull()
  rejectLoad(new Error('Old storage request failed'))
  await settle()
  expect(connection.useConnection().status).toBe('connected')
  expect(connection.cachedApiAuthToken()).toBe('new-token')
  expect(connection.connectionPreference()).toEqual({ mode: 'remote', remoteUrl: 'https://new.example' })
})

test('org-baked cloud build with local disabled auto-connects to cloudUrl and refuses startLocal', async () => {
  const cloudUrl = 'https://jaz.acme.com'
  const { connection } = await fixture({
    cloudUrl,
    localEnabled: false,
    health: () => Response.json({ ok: true, auth_required: false }),
  })
  expect(connection.useConnection().status).toBe('connected')
  expect(connection.useConnection().url).toBe(cloudUrl)
  expect(await connection.startLocal()).toContain('disabled')
})

test('Cloud OIDC endpoint verifies via /v1/auth/me, stores user identity, and surfaces the sign-in requirement without token', async () => {
  const cloudUrl = 'https://jaz-cloud.example'
  let authCheckHits = 0
  const { connection } = await fixture({
    discovery: () => Response.json({
      ok: true,
      mode: 'cloud',
      auth: 'oidc',
      oidc_login_url: '/v1/auth/login',
      capabilities: { spaces: true, connected_machines: true, cloud_instructions: true },
    }),
    auth: () => {
      authCheckHits += 1
      return Response.json({ error: 'unexpected' }, { status: 404 })
    },
    authMe: (init) => {
      if (new Headers(init?.headers).get('Authorization') !== 'Bearer oidc-valid') {
        return Response.json({ error: 'authentication required' }, { status: 401 })
      }
      return Response.json({
        id: 'usr_9',
        org_id: 'org_cas',
        org_name: 'CAS',
        email: 'engineer@cas.example',
        display_name: 'Sam',
        role: 'admin',
      })
    },
  })

  expect(await connection.connectRemote(cloudUrl)).toContain('Sign in to connect')
  expect(connection.useConnection()).toMatchObject({
    status: 'disconnected',
    mode: 'cloud',
    auth: 'oidc',
    oidcLoginUrl: '/v1/auth/login',
    user: null,
  })
  expect(authCheckHits).toBe(0)

  expect(await connection.connectRemote(`${cloudUrl}#token=oidc-valid`)).toBeNull()
  expect(connection.useConnection()).toMatchObject({
    status: 'connected',
    url: cloudUrl,
    mode: 'cloud',
    auth: 'oidc',
    user: {
      id: 'usr_9',
      org_id: 'org_cas',
      org_name: 'CAS',
      email: 'engineer@cas.example',
      display_name: 'Sam',
      role: 'admin',
    },
  })
  expect(connection.isCloudConnection()).toBe(true)
  expect(authCheckHits).toBe(0)
})

test('unsolicited web #token= or ?token= never hijacks connection, while pending #code= exchanges once with state verification', async () => {
  const cloudUrl = 'https://jaz-cloud.example'
  let exchangeHits = 0
  // Unsolicited #token= and #server= without a pending sessionStorage record must be ignored and scrubbed.
  const unsolicited = await fixture({
    web: true,
    locationHref: `https://app.jaz.chat/#token=attacker-token&server=${encodeURIComponent(cloudUrl)}`,
    discovery: () => Response.json({ ok: true, mode: 'cloud', auth: 'oidc', oidc_login_url: '/v1/auth/login' }),
    authMe: () => Response.json({ id: 'usr_evil', org_id: 'org_evil', email: 'evil@example.com', display_name: 'Evil', role: 'admin' }),
  })
  expect(unsolicited.connection.useConnection().status).toBe('disconnected')
  expect(window.location.hash).toBe('')
  unsolicited.connection.disconnectBackend()

  // Unsolicited hashchange hijack is also ignored.
  unsolicited.triggerHashChange(`#token=attacker-token&server=${encodeURIComponent(cloudUrl)}`)
  await settle()
  expect(unsolicited.connection.useConnection().status).toBe('disconnected')
  expect(window.location.hash).toBe('')

  // Valid pending sessionStorage state + #code= callback exchanges code once and stores resource_ticket.
  const pendingState = 'state-valid-123'
  const pendingVerifier = 'verifier-web-123'
  const valid = await fixture({
    web: true,
    locationHref: `https://app.jaz.chat/#code=one-time-code&state=${pendingState}`,
    sessionStoredValues: [[
      'jaz.oidc.pending',
      JSON.stringify({ server: cloudUrl, state: pendingState, codeVerifier: pendingVerifier, startedAt: Date.now() }),
    ]],
    discovery: () => Response.json({
      ok: true,
      mode: 'cloud',
      auth: 'oidc',
      oidc_login_url: '/v1/auth/login',
      capabilities: { spaces: true },
    }),
    authExchange: (init) => {
      exchangeHits += 1
      expect(init.method).toBe('POST')
      expect(JSON.parse(String(init.body))).toEqual({ code: 'one-time-code', code_verifier: pendingVerifier })
      return Response.json({
        token: 'exchanged-session-token',
        user: {
          id: 'usr_web',
          org_id: 'org_web',
          email: 'web@example.com',
          display_name: 'Web User',
          role: 'member',
        },
        resource_ticket: { token: 'rt_web_1', expires_at: '2099-01-01T00:00:00Z' },
      })
    },
  })
  expect(exchangeHits).toBe(1)
  expect(valid.connection.useConnection()).toMatchObject({
    status: 'connected',
    url: cloudUrl,
    mode: 'cloud',
    auth: 'oidc',
    user: { id: 'usr_web', email: 'web@example.com' },
  })
  expect(valid.connection.isCloudApiSession()).toBe(true)
  expect(valid.connection.apiEmbeddedGetUrl('/v1/runs/r1/files/a.txt')).toContain('ticket=rt_web_1')
  expect(window.location.hash).toBe('')
  expect(valid.sessionStored.has('jaz.oidc.pending')).toBe(false)
})

test('Electron OIDC flow invokes beginOidcSignIn, opens jaz://auth/callback login URL with state, and completes via onOidcCallback', async () => {
  const cloudUrl = 'https://jaz-cloud.example'
  let capturedExchangeBody = null
  const f = await fixture({
    discovery: () => Response.json({
      ok: true,
      mode: 'cloud',
      auth: 'oidc',
      oidc_login_url: '/v1/auth/login',
      capabilities: { spaces: true, connected_machines: true },
    }),
    authExchange: (init) => {
      capturedExchangeBody = JSON.parse(String(init.body))
      expect(capturedExchangeBody.code).toBe('desktop-code-1')
      expect(typeof capturedExchangeBody.code_verifier).toBe('string')
      expect(capturedExchangeBody.code_verifier.length).toBeGreaterThanOrEqual(32)
      return Response.json({
        token: 'desktop-oidc-token',
        user: {
          id: 'usr_desk',
          org_id: 'org_desk',
          org_name: 'Desk Org',
          email: 'desk@example.com',
          display_name: 'Desk User',
          role: 'admin',
        },
        resource_ticket: { token: 'rt_desk_1', expires_at: '2099-01-01T00:00:00Z' },
      })
    },
  })

  expect(await f.connection.startCloudSignIn(cloudUrl)).toBeNull()
  expect(f.oidcBeginCalls()).toBe(1)
  expect(f.openedExternal).toHaveLength(1)
  const loginUrl = new URL(f.openedExternal[0])
  expect(loginUrl.origin).toBe(cloudUrl)
  expect(loginUrl.pathname).toBe('/v1/auth/login')
  expect(loginUrl.searchParams.get('client_redirect')).toBe('http://127.0.0.1:43123/auth/callback')
  expect(loginUrl.searchParams.get('code_challenge')).toBeTruthy()
  const state = loginUrl.searchParams.get('state')
  expect(state).toBeTruthy()

  f.triggerElectronOidcCallback(`jaz://auth/callback#code=desktop-code-1&state=${state}`)
  await settle()
  expect(f.connection.useConnection()).toMatchObject({
    status: 'connected',
    url: cloudUrl,
    mode: 'cloud',
    auth: 'oidc',
    user: { id: 'usr_desk', org_name: 'Desk Org', role: 'admin' },
  })
  expect(f.connection.apiEmbeddedGetUrl('/v1/runs/r1/files/doc.pdf')).toContain('ticket=rt_desk_1')
})

test('web Cloud sign-in uses root client_redirect with PKCE from nested routes and disconnect revokes session', async () => {
  const cloudUrl = 'https://jaz-cloud.example'
  const logoutAuthHeaders = []
  const f = await fixture({
    web: true,
    locationHref: 'https://app.jaz.chat/new',
    discovery: () => Response.json({
      ok: true,
      mode: 'cloud',
      auth: 'oidc',
      oidc_login_url: '/v1/auth/login',
    }),
    authMe: () => Response.json({
      id: 'usr_web',
      org_id: 'org_web',
      email: 'web@example.com',
      display_name: 'Web User',
      role: 'member',
    }),
    authLogout: (init) => {
      logoutAuthHeaders.push(new Headers(init?.headers).get('Authorization'))
      return new Response(null, { status: 204 })
    },
  })

  expect(await f.connection.startCloudSignIn(cloudUrl)).toBeNull()
  expect(f.openedExternal).toHaveLength(1)
  const loginUrl = new URL(f.openedExternal[0])
  expect(loginUrl.searchParams.get('client_redirect')).toBe('https://app.jaz.chat/')
  expect(loginUrl.searchParams.get('code_challenge')).toBeTruthy()
  const pending = JSON.parse(f.sessionStored.get('jaz.oidc.pending'))
  expect(pending.codeVerifier).toBeTruthy()
  expect(loginUrl.searchParams.get('code_challenge')).toBe(await f.connection.pkceS256Challenge(pending.codeVerifier))

  expect(await f.connection.connectRemote(`${cloudUrl}#token=oidc-live-tok`)).toBeNull()
  f.connection.disconnectBackend()
  await settle()
  expect(logoutAuthHeaders).toEqual(['Bearer oidc-live-tok'])
})

test('hosted Cloud Web auto-targets same origin via /cloud-config.json, uses cookie auth, and redirects cleanly to /v1/auth/login', async () => {
  const origin = 'https://jaz-cloud-staging.camadvanced.com'
  let cookieAuthed = false
  const apiCalls = []
  const makeOptions = () => ({
    web: true,
    locationHref: `${origin}/`,
    cloudConfig: () => Response.json({ cloud_url: origin, local_enabled: false }),
    discovery: () => Response.json({
      ok: true,
      mode: 'cloud',
      auth: 'oidc',
      oidc_login_url: '/v1/auth/login',
      capabilities: { spaces: true, boards: true, bots: true, loops: true, widgets: true },
    }),
    authMe: (init) => {
      if (init?.credentials !== 'include') {
        return Response.json({ error: 'missing credentials', code: 'unauthorized' }, { status: 401 })
      }
      if (!cookieAuthed) {
        return Response.json({ error: 'authentication required', code: 'unauthorized' }, { status: 401 })
      }
      return Response.json({
        user_id: 'usr_hosted',
        org_id: 'org-default',
        org_name: 'CAS',
        email: 'user@cas.example',
        display_name: 'Hosted User',
        role: 'admin',
        resource_ticket: { token: 'rt_cookie_1', expires_at: '2099-01-01T00:00:00Z' },
      })
    },
    feature: (init) => {
      apiCalls.push({
        credentials: init?.credentials,
        auth: new Headers(init?.headers).get('Authorization'),
      })
      if (!cookieAuthed) {
        return Response.json({ error: 'session revoked', code: 'unauthorized' }, { status: 401 })
      }
      return Response.json({ threads: [] })
    },
  })

  // Unauthenticated visit lands on OIDC sign-in state for the hosted origin (never local).
  const unauthed = await fixture(makeOptions())
  expect(unauthed.connection.useConnection()).toMatchObject({
    status: 'disconnected',
    url: origin,
    mode: 'cloud',
    auth: 'oidc',
    oidcLoginUrl: '/v1/auth/login',
    error: 'Sign in to connect to this server',
  })
  expect(await unauthed.connection.startLocal()).toContain('disabled')

  // Clicking Sign in on same-origin hosted web navigates straight to /v1/auth/login without client_redirect.
  expect(await unauthed.connection.startCloudSignIn(origin, '/v1/auth/login')).toBeNull()
  expect(unauthed.openedExternal).toEqual([`${origin}/v1/auth/login`])
  expect(unauthed.sessionStored.has('jaz.oidc.pending')).toBe(false)

  // After OIDC callback sets __Host-jaz_session and redirects to /, reloading auto-connects via cookie.
  cookieAuthed = true
  const authed = await fixture(makeOptions())
  expect(authed.connection.useConnection()).toMatchObject({
    status: 'connected',
    url: origin,
    mode: 'cloud',
    auth: 'oidc',
    user: { id: 'usr_hosted', org_id: 'org-default', org_name: 'CAS' },
  })
  expect(authed.connection.cachedApiAuthToken(origin)).toBe('')
  expect(authed.connection.apiEmbeddedGetUrl('/v1/runs/r1/files/report.pdf')).toContain('ticket=rt_cookie_1')

  // Same-origin Cloud API requests carry credentials: 'include' and no Authorization header.
  const res = await authed.connection.apiFetch('/v1/threads')
  expect(res.ok).toBe(true)
  expect(apiCalls).toEqual([{ credentials: 'include', auth: null }])

  // Revoking the cookie drops back to OIDC sign-in while preserving cloud metadata.
  cookieAuthed = false
  await authed.connection.apiFetch('/v1/threads').catch(() => null)
  expect(authed.connection.useConnection()).toMatchObject({
    status: 'disconnected',
    url: origin,
    mode: 'cloud',
    auth: 'oidc',
    oidcLoginUrl: '/v1/auth/login',
    error: 'Session expired. Sign in again.',
  })
})

test('hosted cloud web build (VITE_JAZ_LOCAL_ENABLED=false, empty cloudUrl) defaults cloudUrl and apiBaseUrl to window.location.origin', async () => {
  const hostedBundle = await build({
    entrypoints: [new URL('./clientRuntime.ts', import.meta.url).pathname],
    target: 'browser',
    format: 'cjs',
    define: {
      'import.meta.env': JSON.stringify({
        VITE_JAZ_LOCAL_ENABLED: 'false',
        VITE_JAZ_CLOUD_URL: '',
      }),
    },
  })
  if (!hostedBundle.success) {
    throw new AggregateError(hostedBundle.logs)
  }
  replace('window', {
    location: { origin: 'https://jaz-cloud-staging.camadvanced.com' },
    jaz: undefined,
  })
  replace('navigator', { platform: 'MacIntel' })
  const mod = { exports: {} }
  new Function('module', 'exports', await hostedBundle.outputs[0].text())(mod, mod.exports)
  expect(mod.exports.clientRuntime.kind).toBe('web')
  expect(mod.exports.clientRuntime.capabilities.localBackend).toBe(false)
  expect(mod.exports.clientRuntime.cloudUrl).toBe('https://jaz-cloud-staging.camadvanced.com')
  expect(mod.exports.clientRuntime.defaultApiBaseUrl()).toBe('https://jaz-cloud-staging.camadvanced.com')
})

test('concurrent Cloud 401s deduplicate into a single POST /v1/auth/refresh without replay revocation, rotate persisted tokens, and send refresh_token on logout', async () => {
  const cloudUrl = 'https://jaz-cloud.example'
  let activeToken = 'jwt_1'
  let activeRefresh = 'rt_1'
  const usedRefreshTokens = new Set()
  let familyRevoked = false
  let refreshHits = 0
  let releaseRefresh
  let refreshStarted
  const refreshGate = new Promise((resolve) => {
    releaseRefresh = resolve
  })
  const refreshEntered = new Promise((resolve) => {
    refreshStarted = resolve
  })
  let releaseSlowOldRequest
  const slowOldRequestGate = new Promise((resolve) => {
    releaseSlowOldRequest = resolve
  })
  const logoutPayloads = []

  const pendingState = 'state-refresh-1'
  const pendingVerifier = 'verifier-refresh-1'
  const f = await fixture({
    web: true,
    locationHref: `https://app.jaz.chat/#code=code-1&state=${pendingState}`,
    sessionStoredValues: [[
      'jaz.oidc.pending',
      JSON.stringify({ server: cloudUrl, state: pendingState, codeVerifier: pendingVerifier, startedAt: Date.now() }),
    ]],
    discovery: () => Response.json({
      ok: true,
      mode: 'cloud',
      auth: 'oidc',
      oidc_login_url: '/v1/auth/login',
    }),
    authExchange: () => Response.json({
      token: 'jwt_1',
      expires_at: '2099-01-01T00:00:00Z',
      refresh_token: 'rt_1',
      refresh_expires_at: '2099-02-01T00:00:00Z',
      user: { user_id: 'usr_1', org_id: 'org_1', email: 'u@cas.example', display_name: 'User', role: 'member' },
      resource_ticket: { token: 'ticket_1', expires_at: '2099-01-01T00:00:00Z' },
    }),
    authRefresh: async (init) => {
      refreshHits += 1
      const body = JSON.parse(String(init?.body || '{}'))
      if (familyRevoked || usedRefreshTokens.has(body.refresh_token) || body.refresh_token !== activeRefresh) {
        familyRevoked = true
        return Response.json({ error: 'refresh token revoked or reused', code: 'unauthorized' }, { status: 401 })
      }
      usedRefreshTokens.add(body.refresh_token)
      refreshStarted()
      await refreshGate
      activeToken = 'jwt_2'
      activeRefresh = 'rt_2'
      return Response.json({
        token: activeToken,
        expires_at: '2099-01-02T00:00:00Z',
        refresh_token: activeRefresh,
        refresh_expires_at: '2099-02-02T00:00:00Z',
        user: { user_id: 'usr_1', org_id: 'org_1', email: 'u@cas.example', display_name: 'User Rotated', role: 'admin' },
        resource_ticket: { token: 'ticket_2', expires_at: '2099-01-02T00:00:00Z' },
      })
    },
    authLogout: (init) => {
      logoutPayloads.push({
        auth: new Headers(init?.headers).get('Authorization'),
        body: init?.body ? JSON.parse(String(init.body)) : null,
      })
      return Response.json({ ok: true })
    },
    feature: async (init, target) => {
      const auth = new Headers(init?.headers).get('Authorization')
      if (target.pathname === '/v1/slow-stale' && auth === 'Bearer jwt_1') {
        await slowOldRequestGate
        return Response.json({ error: 'invalid session token', code: 'unauthorized' }, { status: 401 })
      }
      if (auth !== `Bearer ${activeToken}`) {
        return Response.json({ error: 'invalid session token', code: 'unauthorized' }, { status: 401 })
      }
      return Response.json({ ok: true, path: target.pathname, auth })
    },
  })

  expect(f.connection.useConnection().status).toBe('connected')
  expect(f.browserCredentials.get(cloudUrl)).toEqual({ token: 'jwt_1', refreshToken: 'rt_1' })

  // Expire jwt_1 on server and launch 4 concurrent requests + 1 slow request that arrives AFTER refresh completes.
  activeToken = 'expired-on-server'
  const slowStale = f.connection.apiFetch('/v1/slow-stale')
  const concurrent = Promise.all([
    f.connection.apiFetch('/v1/a'),
    f.connection.apiFetch('/v1/b'),
    f.connection.apiFetch('/v1/c'),
    f.connection.apiFetch('/v1/d'),
  ])

  await refreshEntered
  releaseRefresh()
  const results = await concurrent
  expect(refreshHits).toBe(1)
  expect(familyRevoked).toBe(false)
  for (const res of results) {
    expect(res.ok).toBe(true)
    expect((await res.json()).auth).toBe('Bearer jwt_2')
  }

  // Now release the late-arriving 401 that was sent with jwt_1 before refresh started: must NOT trigger a 2nd refresh!
  releaseSlowOldRequest()
  const slowRes = await slowStale
  expect(slowRes.ok).toBe(true)
  expect((await slowRes.json()).auth).toBe('Bearer jwt_2')
  expect(refreshHits).toBe(1)
  expect(familyRevoked).toBe(false)

  await settle()
  expect(f.connection.cachedApiAuthToken(cloudUrl)).toBe('jwt_2')
  expect(f.connection.cachedApiRefreshToken(cloudUrl)).toBe('rt_2')
  expect(f.connection.useConnection().user).toMatchObject({ display_name: 'User Rotated', role: 'admin' })
  expect(f.connection.apiEmbeddedGetUrl('/v1/runs/r1/files/x.txt')).toContain('ticket=ticket_2')
  expect(f.browserCredentials.get(cloudUrl)).toEqual({ token: 'jwt_2', refreshToken: 'rt_2' })

  f.connection.disconnectBackend()
  await settle()
  expect(logoutPayloads).toEqual([{ auth: 'Bearer jwt_2', body: { refresh_token: 'rt_2' } }])
})

test('hosted Web cookie mode refreshes seamlessly on startup and deduplicates concurrent 401s while staying on cookies', async () => {
  const origin = 'https://jaz-cloud-staging.camadvanced.com'
  let sessionCookieValid = false
  let refreshCookieValid = true
  let refreshHits = 0
  let releaseRefresh
  let onRefreshStarted
  let refreshGate = null

  const f = await fixture({
    web: true,
    locationHref: `${origin}/`,
    cloudConfig: () => Response.json({ cloud_url: origin, local_enabled: false }),
    discovery: () => Response.json({
      ok: true,
      mode: 'cloud',
      auth: 'oidc',
      oidc_login_url: '/v1/auth/login',
    }),
    authMe: (init) => {
      if (init?.credentials !== 'include' || !sessionCookieValid) {
        return Response.json({ error: 'invalid session token', code: 'unauthorized' }, { status: 401 })
      }
      return Response.json({
        user_id: 'usr_cookie',
        org_id: 'org_cas',
        email: 'cookie@cas.example',
        display_name: 'Cookie User',
        role: 'member',
        resource_ticket: { token: 'rt_me', expires_at: '2099-01-01T00:00:00Z' },
      })
    },
    authRefresh: async (init) => {
      refreshHits += 1
      expect(init?.credentials).toBe('include')
      expect(JSON.parse(String(init?.body || '{}'))).toEqual({})
      if (!refreshCookieValid) {
        return Response.json({ error: 'refresh token revoked', code: 'unauthorized' }, { status: 401 })
      }
      if (refreshGate) {
        onRefreshStarted?.()
        await refreshGate
      }
      sessionCookieValid = true
      return Response.json({
        token: 'cookie-jwt-unused-in-js',
        refresh_token: 'rt_cookie_unused_in_js',
        user: { user_id: 'usr_cookie', org_id: 'org_cas', email: 'cookie@cas.example', display_name: 'Cookie User', role: 'member' },
        resource_ticket: { token: `rt_cookie_${refreshHits}`, expires_at: '2099-01-01T00:00:00Z' },
      })
    },
    feature: (init) => {
      expect(init?.credentials).toBe('include')
      expect(new Headers(init?.headers).get('Authorization')).toBeNull()
      if (!sessionCookieValid) {
        return Response.json({ error: 'invalid session token', code: 'unauthorized' }, { status: 401 })
      }
      return Response.json({ ok: true })
    },
  })

  // Startup recovered from expired 24h session cookie using the 30d refresh cookie without switching to bearer auth.
  expect(refreshHits).toBe(1)
  expect(f.connection.useConnection()).toMatchObject({ status: 'connected', url: origin, mode: 'cloud', auth: 'oidc' })
  expect(f.connection.cachedApiAuthToken(origin)).toBe('')
  expect(f.connection.apiEmbeddedGetUrl('/v1/runs/r1/files/a.txt')).toContain('ticket=rt_cookie_1')

  // Expire session cookie mid-session and fire 4 concurrent requests -> exactly 1 refresh call.
  sessionCookieValid = false
  const started = new Promise((resolve) => { onRefreshStarted = resolve })
  refreshGate = new Promise((resolve) => { releaseRefresh = resolve })
  const batch = Promise.all([
    f.connection.apiFetch('/v1/t1'),
    f.connection.apiFetch('/v1/t2'),
    f.connection.apiFetch('/v1/t3'),
    f.connection.apiFetch('/v1/t4'),
  ])
  await started
  releaseRefresh()
  const responses = await batch
  expect(refreshHits).toBe(2)
  expect(responses.every((r) => r.ok)).toBe(true)
  expect(f.connection.cachedApiAuthToken(origin)).toBe('')
  expect(f.connection.apiEmbeddedGetUrl('/v1/runs/r1/files/a.txt')).toContain('ticket=rt_cookie_2')
})

test('transient 503 refresh error preserves Cloud session while 401 refresh failure cleanly logs out and forgets dead credentials', async () => {
  const cloudUrl = 'https://jaz-cloud.example'
  let refreshStatus = 503
  let refreshHits = 0
  const pendingState = 'state-err-1'

  const f = await fixture({
    web: true,
    locationHref: `https://app.jaz.chat/#code=code-err&state=${pendingState}`,
    sessionStoredValues: [[
      'jaz.oidc.pending',
      JSON.stringify({ server: cloudUrl, state: pendingState, codeVerifier: 'ver-err-1', startedAt: Date.now() }),
    ]],
    discovery: () => Response.json({ ok: true, mode: 'cloud', auth: 'oidc', oidc_login_url: '/v1/auth/login' }),
    authExchange: () => Response.json({
      token: 'jwt_initial',
      refresh_token: 'rt_initial',
      user: { user_id: 'usr_1', org_id: 'org_1', email: 'u@cas.example', display_name: 'User', role: 'member' },
    }),
    authRefresh: () => {
      refreshHits += 1
      if (refreshStatus === 503) {
        return Response.json({ error: 'temporarily unavailable' }, { status: 503 })
      }
      return Response.json({ error: 'refresh token revoked', code: 'unauthorized' }, { status: 401 })
    },
    feature: () => Response.json({ error: 'invalid session token', code: 'unauthorized' }, { status: 401 }),
  })

  expect(f.connection.useConnection().status).toBe('connected')

  // 503 on /v1/auth/refresh must NOT log the user out or erase the refresh token.
  const transient = await f.connection.apiFetch('/v1/threads')
  expect(transient.status).toBe(401)
  expect(refreshHits).toBe(1)
  expect(f.connection.useConnection().status).toBe('connected')
  expect(f.connection.cachedApiRefreshToken(cloudUrl)).toBe('rt_initial')
  expect(f.browserCredentials.has(cloudUrl)).toBe(true)

  // 401 on /v1/auth/refresh must cleanly log out, clear tokens, forget dead browser credential, and preserve OIDC metadata.
  refreshStatus = 401
  const expired = await f.connection.apiFetch('/v1/threads')
  expect(expired.status).toBe(401)
  await settle()
  expect(refreshHits).toBe(2)
  expect(f.connection.useConnection()).toMatchObject({
    status: 'disconnected',
    mode: 'cloud',
    auth: 'oidc',
    oidcLoginUrl: '/v1/auth/login',
    error: 'Session expired. Sign in again.',
    user: null,
  })
  expect(f.connection.cachedApiAuthToken(cloudUrl)).toBe('')
  expect(f.connection.cachedApiRefreshToken(cloudUrl)).toBe('')
  expect(f.browserCredentials.has(cloudUrl)).toBe(false)
})

test('browser restore transparently refreshes an expired saved Cloud access token using its saved refresh token', async () => {
  const cloudUrl = 'https://jaz-cloud.example'
  const browserCredentials = new Map([[cloudUrl, { token: 'jwt_old', refreshToken: 'rt_saved_1' }]])
  let refreshHits = 0

  const f = await fixture({
    web: true,
    browserCredentials,
    storedValues: [['jaz.connection', JSON.stringify({ mode: 'remote', remoteUrl: cloudUrl })]],
    discovery: () => Response.json({ ok: true, mode: 'cloud', auth: 'oidc', oidc_login_url: '/v1/auth/login' }),
    authMe: (init) => {
      if (new Headers(init?.headers).get('Authorization') === 'Bearer jwt_fresh') {
        return Response.json({ user_id: 'usr_1', org_id: 'org_1', email: 'u@cas.example', display_name: 'User', role: 'member' })
      }
      return Response.json({ error: 'token expired', code: 'unauthorized' }, { status: 401 })
    },
    authRefresh: (init) => {
      refreshHits += 1
      expect(JSON.parse(String(init?.body || '{}'))).toEqual({ refresh_token: 'rt_saved_1' })
      return Response.json({
        token: 'jwt_fresh',
        refresh_token: 'rt_saved_2',
        user: { user_id: 'usr_1', org_id: 'org_1', email: 'u@cas.example', display_name: 'User', role: 'member' },
        resource_ticket: { token: 'rt_restored', expires_at: '2099-01-01T00:00:00Z' },
      })
    },
  })

  expect(refreshHits).toBe(1)
  expect(f.connection.useConnection()).toMatchObject({ status: 'connected', url: cloudUrl, mode: 'cloud', auth: 'oidc' })
  expect(f.connection.cachedApiAuthToken(cloudUrl)).toBe('jwt_fresh')
  expect(f.connection.cachedApiRefreshToken(cloudUrl)).toBe('rt_saved_2')
  expect(browserCredentials.get(cloudUrl)).toEqual({ token: 'jwt_fresh', refreshToken: 'rt_saved_2' })
})
