import { afterEach, expect, test } from 'bun:test'
import { build } from 'bun'
import { createRequire } from 'node:module'
import * as React from 'react'

const bundle = await build({
  entrypoints: ['cloud-account-fixture'],
  target: 'browser',
  format: 'cjs',
  external: ['react', '@/lib/browserCredentials'],
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
          `export * from ${JSON.stringify(new URL('./api/cloudClient.ts', import.meta.url).pathname)}`,
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
  storedValues = [],
  sessionStoredValues = [],
  encryptCredential = (value) => `sealed:${value}`,
  decryptCredential = (value) => value.replace(/^sealed:/, ''),
  cloudDiscovery,
  cloudExchange,
  cloudAuthMe,
  cloudLogout,
  cloudFeature,
  startLocalBackend = async () => ({ ok: true, url: 'http://127.0.0.1:5299', key: 'local-key' }),
  web = false,
  locationHref = 'http://localhost/',
  browserCredentials = new Map(),
} = {}) {
  const stored = new Map(storedValues)
  const sessionStored = new Map(sessionStoredValues)
  const openedExternal = []
  let oidcBeginCalls = 0
  let oidcCallbackHandler = null
  const loc = new URL(locationHref)
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
      const result = await startLocalBackend()
      if (!result?.ok) throw new Error(result?.error || 'Local start failed')
      return { url: result.url, token: 'local-bearer', remembered: false }
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
      replaceState(_s, _t, next) {
        const updated = new URL(String(next))
        window.location.href = updated.href
        window.location.hash = updated.hash
      },
    },
    addEventListener() {},
    open(target) {
      openedExternal.push(String(target))
    },
    jaz: web ? undefined : {
      apiBaseUrl: 'http://127.0.0.1:5299',
      localEnabled: true,
      windowKind: 'main',
      connectionSession: session,
      encryptCredential,
      decryptCredential,
      startLocalBackend,
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
      throw new Error(`Unexpected local request ${target.pathname}`)
    }
    if (target.pathname === '/.well-known/jaz-server') {
      return cloudDiscovery
        ? cloudDiscovery(target)
        : Response.json({
            ok: true,
            mode: 'cloud',
            auth: 'oidc',
            oidc_login_url: '/v1/auth/login',
            capabilities: { spaces: true, connected_machines: true, cloud_instructions: true },
          })
    }
    if (target.pathname === '/v1/auth/exchange' && cloudExchange) {
      return cloudExchange(init)
    }
    if (target.pathname === '/v1/auth/me' && cloudAuthMe) {
      return cloudAuthMe(init)
    }
    if (target.pathname === '/v1/auth/logout') {
      return cloudLogout ? cloudLogout(init) : new Response(null, { status: 204 })
    }
    if (cloudFeature) {
      return cloudFeature(target, init)
    }
    throw new Error(`Unexpected cloud request ${url}`)
  })

  const module = { exports: {} }
  evaluate(
    (name) =>
      name === 'react'
        ? { ...React, useSyncExternalStore: (_sub, snap) => snap(), useEffect: (effect) => effect() }
        : name === '@/lib/browserCredentials'
          ? {
              loadBrowserCredential: async (key) => browserCredentials.get(key),
              saveBrowserCredential: async (key, token, current) => {
                if (!current()) throw new Error('Connection changed')
                browserCredentials.set(key, { token })
              },
              forgetBrowserCredential: async (key) => browserCredentials.delete(key),
            }
          : require(name),
    module,
    module.exports,
  )
  activeModule = module.exports
  await activeModule.waitForCloudRestore()
  await settle()
  return {
    mod: activeModule,
    stored,
    sessionStored,
    openedExternal,
    oidcBeginCalls: () => oidcBeginCalls,
    triggerOidcCallback: (url) => oidcCallbackHandler?.(url),
    browserCredentials,
  }
}

test('attached Cloud OIDC sign-in, reload restore, 401 expiry, and sign-out keep Local primary connected', async () => {
  const cloudUrl = 'https://jaz-cloud.example'
  const logoutHeaders = []
  let exchangedVerifier = ''
  const f = await fixture({
    cloudExchange: (init) => {
      const body = JSON.parse(String(init.body))
      exchangedVerifier = body.code_verifier
      expect(body.code).toBe('attach-code-1')
      return Response.json({
        token: 'cloud-jwt-1',
        user: {
          id: 'usr_1',
          org_id: 'org_acme',
          org_name: 'Acme Cloud',
          email: 'member@acme.example',
          display_name: 'Alex',
          role: 'admin',
        },
        resource_ticket: { token: 'rt_attached_1', expires_at: '2099-01-01T00:00:00Z' },
      })
    },
    cloudLogout: (init) => {
      logoutHeaders.push(new Headers(init?.headers).get('Authorization'))
      return new Response(null, { status: 204 })
    },
  })

  // Primary is connected to Local.
  expect(f.mod.useConnection()).toMatchObject({
    status: 'connected',
    url: 'http://127.0.0.1:5299',
    mode: 'local',
  })
  expect(f.mod.activeCloudTarget()).toBeNull()

  // Attach Jaz Cloud via loopback OIDC + PKCE.
  expect(await f.mod.attachCloudSignIn(cloudUrl)).toBeNull()
  expect(f.oidcBeginCalls()).toBe(1)
  expect(f.openedExternal).toHaveLength(1)
  const loginUrl = new URL(f.openedExternal[0])
  expect(loginUrl.origin).toBe(cloudUrl)
  expect(loginUrl.searchParams.get('client_redirect')).toBe('http://127.0.0.1:45555/auth/callback')
  const state = loginUrl.searchParams.get('state')
  const pending = JSON.parse(f.sessionStored.get('jaz.oidc.pending'))
  expect(pending.purpose).toBe('attached')
  expect(loginUrl.searchParams.get('code_challenge')).toBe(await f.mod.pkceS256Challenge(pending.codeVerifier))

  // Incoming OIDC callback routes to attached Cloud without touching Local!
  f.triggerOidcCallback(`jaz://auth/callback#code=attach-code-1&state=${state}`)
  await settle()

  expect(exchangedVerifier).toBe(pending.codeVerifier)
  expect(f.mod.useConnection()).toMatchObject({
    status: 'connected',
    url: 'http://127.0.0.1:5299',
    mode: 'local',
  })
  expect(f.mod.apiBaseUrl()).toBe('http://127.0.0.1:5299')
  expect(f.mod.isCloudApiSession()).toBe(false)
  expect(f.mod.currentCloudAccount()).toMatchObject({
    status: 'connected',
    url: cloudUrl,
    user: { id: 'usr_1', org_name: 'Acme Cloud' },
  })
  expect(f.mod.activeCloudTarget()).toEqual({
    baseUrl: cloudUrl,
    token: 'cloud-jwt-1',
    ticket: { token: 'rt_attached_1', expires_at: '2099-01-01T00:00:00Z' },
    user: {
      id: 'usr_1',
      org_id: 'org_acme',
      org_name: 'Acme Cloud',
      email: 'member@acme.example',
      display_name: 'Alex',
      role: 'admin',
    },
  })
  expect(f.stored.get('jaz.cloudAccount.url')).toBe(cloudUrl)
  expect(f.stored.get(`jaz.cloudAccount.auth.${cloudUrl}`)).toBe('enc:v1:sealed:cloud-jwt-1')

  // Fresh reload restores attached Cloud alongside Local via /v1/auth/me.
  const persistedEntries = [...f.stored.entries()]
  f.mod.disconnectBackend()
  let authMeBearer = ''
  let featureStatus = 403
  const reloaded = await fixture({
    storedValues: persistedEntries,
    cloudAuthMe: (init) => {
      authMeBearer = new Headers(init?.headers).get('Authorization') ?? ''
      return Response.json({
        id: 'usr_1',
        org_id: 'org_acme',
        org_name: 'Acme Cloud',
        email: 'member@acme.example',
        display_name: 'Alex',
        role: 'admin',
        resource_ticket: { token: 'rt_restored_2', expires_at: '2099-01-01T00:00:00Z' },
      })
    },
    cloudFeature: () =>
      featureStatus === 403
        ? Response.json({ error: 'admin required' }, { status: 403 })
        : Response.json({ error: 'expired' }, { status: 401 }),
    cloudLogout: (init) => {
      logoutHeaders.push(new Headers(init?.headers).get('Authorization'))
      return new Response(null, { status: 204 })
    },
  })

  expect(authMeBearer).toBe('Bearer cloud-jwt-1')
  expect(reloaded.mod.useConnection()).toMatchObject({ status: 'connected', mode: 'local' })
  expect(reloaded.mod.currentCloudAccount()).toMatchObject({ status: 'connected', url: cloudUrl })
  expect(reloaded.mod.cloudEmbeddedGetUrl('/v1/runs/r1/files/a.txt')).toBe(
    `${cloudUrl}/v1/runs/r1/files/a.txt?ticket=rt_restored_2`,
  )

  // Feature-level 403 keeps attached Cloud connected; 401 disconnects ONLY Cloud, not Local.
  const forbidden = await reloaded.mod.cloudFetch('/v1/instructions/org/org_acme/agents')
  expect(forbidden.status).toBe(403)
  expect(reloaded.mod.currentCloudAccount().status).toBe('connected')

  reloaded.mod.registerCloudThread('thr_cloud_1')
  expect(reloaded.mod.isCloudThread('thr_cloud_1')).toBe(true)

  featureStatus = 401
  const expired = await reloaded.mod.cloudFetch('/v1/threads')
  expect(expired.status).toBe(401)
  expect(reloaded.mod.currentCloudAccount().status).toBe('disconnected')
  expect(reloaded.mod.currentCloudAccount().error).toContain('expired')
  expect(reloaded.mod.activeCloudTarget()).toBeNull()
  expect(reloaded.mod.isCloudThread('thr_cloud_1')).toBe(false)
  expect(reloaded.mod.useConnection()).toMatchObject({ status: 'connected', mode: 'local' })

  // Re-attach and sign out: revokes /v1/auth/logout, clears storage, keeps Local connected.
  reloaded.mod.setAttachedCloudSession({
    baseUrl: cloudUrl,
    token: 'cloud-jwt-2',
    user: { id: 'usr_1', org_id: 'org_acme', email: 'member@acme.example', display_name: 'Alex', role: 'admin' },
  })
  reloaded.mod.registerCloudThread('thr_cloud_2')
  reloaded.mod.signOutCloudAccount()
  await settle()

  expect(logoutHeaders).toEqual(['Bearer cloud-jwt-2'])
  expect(reloaded.mod.activeCloudTarget()).toBeNull()
  expect(reloaded.mod.isCloudThread('thr_cloud_2')).toBe(false)
  expect(reloaded.stored.has('jaz.cloudAccount.url')).toBe(false)
  expect(reloaded.stored.has(`jaz.cloudAccount.auth.${cloudUrl}`)).toBe(false)
  expect(reloaded.mod.useConnection()).toMatchObject({ status: 'connected', mode: 'local' })
})
