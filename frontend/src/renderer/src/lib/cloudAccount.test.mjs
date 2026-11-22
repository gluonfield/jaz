import { afterEach, expect, test } from 'bun:test'
import { build } from 'bun'
import { createRequire } from 'node:module'
import * as React from 'react'

const bundle = await build({
  entrypoints: ['cloud-account-fixture'],
  target: 'browser',
  format: 'cjs',
  external: ['react'],
  define: { 'import.meta.env': '{}' },
  plugins: [{
    name: 'cloud-account-fixture',
    setup(builder) {
      builder.onResolve({ filter: /^cloud-account-fixture$/ }, () => ({ path: 'entry', namespace: 'fixture' }))
      builder.onResolve({ filter: /^\//, namespace: 'fixture' }, (args) => ({ path: args.path, namespace: 'file' }))
      builder.onLoad({ filter: /.*/, namespace: 'fixture' }, () => ({
        contents: [
          `export * from ${JSON.stringify(new URL('./connection.ts', import.meta.url).pathname)}`,
          `export * from ${JSON.stringify(new URL('./cloudAccount.ts', import.meta.url).pathname)}`,
          `export * from ${JSON.stringify(new URL('./api/client.ts', import.meta.url).pathname)}`,
          `export * from ${JSON.stringify(new URL('./api/cloudAuth.ts', import.meta.url).pathname)}`,
        ].join('\n'),
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
let activeModule

function replace(name, value) {
  if (!originals.has(name)) {
    originals.set(name, Object.getOwnPropertyDescriptor(globalThis, name))
  }
  Object.defineProperty(globalThis, name, { configurable: true, writable: true, value })
}

afterEach(() => {
  activeModule?.signOutCloudAccount()
  activeModule?.disconnectBackend()
  activeModule = undefined
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

async function fixture({
  cloudStatusResponse = { connected: false, status: 'disconnected' },
  onLocalCloudConnect,
  onLocalCloudDisconnect,
  onLocalCloudSync,
} = {}) {
  const stored = new Map()
  const sessionStored = new Map()
  const openedExternal = []
  let oidcBeginCalls = 0
  let oidcCallbackHandler = null
  const loc = new URL('http://localhost/')
  let snapshot = { revision: 0, phase: 'initial', url: '', session: null, error: null, localPending: false }
  const session = {
    read: () => ({ document: 1, owner: true, snapshot }),
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
      return { url: 'http://127.0.0.1:5299', token: 'local-bearer', remembered: false }
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
    history: { state: null, replaceState() {} },
    addEventListener() {},
    open(target) {
      openedExternal.push(String(target))
    },
    jaz: {
      apiBaseUrl: 'http://127.0.0.1:5299',
      localEnabled: true,
      windowKind: 'main',
      connectionSession: session,
      startLocalBackend: async () => ({ ok: true, url: 'http://127.0.0.1:5299', key: 'local-key' }),
      beginOidcSignIn: async () => {
        oidcBeginCalls += 1
        return { redirectUri: 'http://127.0.0.1:45555/auth/callback' }
      },
      onOidcCallback: (handler) => {
        oidcCallbackHandler = handler
        return () => {
          if (oidcCallbackHandler === handler) oidcCallbackHandler = null
        }
      },
      openExternalURL: (target) => openedExternal.push(String(target)),
    },
  })
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
  replace('setTimeout', () => 1)
  replace('clearTimeout', () => {})
  replace('fetch', async (url, init) => {
    const target = new URL(String(url))
    if (target.origin === 'http://127.0.0.1:5299') {
      if (target.pathname === '/.well-known/jaz-server') {
        return Response.json({ ok: true, mode: 'local', auth: 'none', capabilities: { session_file_read: true } })
      }
      if (target.pathname === '/health') {
        return Response.json({ ok: true, auth_required: false })
      }
      if (target.pathname === '/v1/cloud/status') {
        return Response.json(cloudStatusResponse)
      }
      if (target.pathname === '/v1/cloud/connect' && onLocalCloudConnect) {
        return onLocalCloudConnect(init)
      }
      if (target.pathname === '/v1/cloud/disconnect') {
        return onLocalCloudDisconnect ? onLocalCloudDisconnect(init) : Response.json({ connected: false, status: 'disconnected' })
      }
      if (target.pathname === '/v1/cloud/sync' && onLocalCloudSync) {
        return onLocalCloudSync(init)
      }
      throw new Error(`Unexpected local request ${target.pathname}`)
    }
    if (target.pathname === '/.well-known/jaz-server') {
      return Response.json({
        ok: true,
        mode: 'cloud',
        auth: 'oidc',
        oidc_login_url: '/v1/auth/login',
        capabilities: { spaces: true, connected_machines: true, cloud_instructions: true },
      })
    }
    throw new Error(`Desktop should never send API requests directly to ${url}`)
  })

  const module = { exports: {} }
  evaluate(
    (name) =>
      name === 'react'
        ? { ...React, useSyncExternalStore: (_sub, snap) => snap(), useEffect: (effect) => effect() }
        : require(name),
    module,
    module.exports,
  )
  activeModule = module.exports
  await settle()
  return {
    mod: activeModule,
    stored,
    sessionStored,
    openedExternal,
    oidcBeginCalls: () => oidcBeginCalls,
    triggerOidcCallback: (url) => oidcCallbackHandler?.(url),
  }
}

test('Desktop Cloud OIDC hands PKCE code to local Go backend /v1/cloud/connect without storing secrets in localStorage', async () => {
  const cloudUrl = 'https://jaz-cloud.example'
  let connectPayload = null
  let disconnectCalls = 0
  let syncCalls = 0

  const f = await fixture({
    onLocalCloudConnect: (init) => {
      connectPayload = JSON.parse(String(init.body))
      return Response.json({
        connected: true,
        status: 'connected',
        url: cloudUrl,
        oidc_login_url: '/v1/auth/login',
        last_sync_at: '2026-11-22T18:00:00Z',
        user: {
          id: 'usr_1',
          org_id: 'org_acme',
          org_name: 'Acme Cloud',
          email: 'member@acme.example',
          display_name: 'Alex',
          role: 'admin',
        },
        capabilities: { spaces: true },
      })
    },
    onLocalCloudSync: () => {
      syncCalls += 1
      return Response.json({
        connected: true,
        status: 'connected',
        url: cloudUrl,
        last_sync_at: '2026-11-22T18:05:00Z',
        user: { id: 'usr_1', org_id: 'org_acme', org_name: 'Acme Cloud', email: 'member@acme.example', display_name: 'Alex', role: 'admin' },
      })
    },
    onLocalCloudDisconnect: () => {
      disconnectCalls += 1
      return Response.json({ connected: false, status: 'disconnected' })
    },
  })

  expect(f.mod.useConnection()).toMatchObject({ status: 'connected', url: 'http://127.0.0.1:5299', mode: 'local' })
  expect(f.mod.currentCloudAccount().status).toBe('disconnected')

  expect(await f.mod.attachCloudSignIn(cloudUrl)).toBeNull()
  expect(f.oidcBeginCalls()).toBe(1)
  const loginUrl = new URL(f.openedExternal[0])
  const state = loginUrl.searchParams.get('state')
  const pending = JSON.parse(f.sessionStored.get('jaz.oidc.pending'))

  f.triggerOidcCallback(`jaz://auth/callback#code=attach-code-1&state=${state}`)
  await settle()

  // Local Go backend received the code + verifier; zero Cloud tokens stored in localStorage.
  expect(connectPayload).toEqual({
    url: cloudUrl,
    code: 'attach-code-1',
    code_verifier: pending.codeVerifier,
  })
  expect([...f.stored.keys()].filter((k) => k.includes('cloudAccount'))).toEqual([])
  expect(f.mod.currentCloudAccount()).toMatchObject({
    status: 'connected',
    url: cloudUrl,
    lastSyncAt: '2026-11-22T18:00:00Z',
    user: { id: 'usr_1', org_name: 'Acme Cloud' },
  })

  // Manual sync hits local Go backend /v1/cloud/sync.
  expect(await f.mod.syncCloudAccount()).toBeNull()
  expect(syncCalls).toBe(1)
  expect(f.mod.currentCloudAccount().lastSyncAt).toBe('2026-11-22T18:05:00Z')

  // Sign out hits local Go backend /v1/cloud/disconnect and keeps Local connected.
  f.mod.signOutCloudAccount()
  await settle()
  expect(disconnectCalls).toBe(1)
  expect(f.mod.currentCloudAccount().status).toBe('disconnected')
  expect(f.mod.useConnection()).toMatchObject({ status: 'connected', mode: 'local' })
})
