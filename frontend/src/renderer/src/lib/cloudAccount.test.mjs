import { afterEach, expect, test } from 'bun:test'
import { build } from 'bun'
import { createRequire } from 'node:module'
import * as React from 'react'

const bundle = await build({
  entrypoints: ['cloud-account-fixture'],
  target: 'browser',
  format: 'cjs',
  external: ['react', '@tanstack/react-query'],
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
          `export * from ${JSON.stringify(new URL('./cloudSession.ts', import.meta.url).pathname)}`,
          `export * from ${JSON.stringify(new URL('./api/client.ts', import.meta.url).pathname)}`,
          `export * from ${JSON.stringify(new URL('./api/cloudAuth.ts', import.meta.url).pathname)}`,
          `export { queryClient } from ${JSON.stringify(new URL('./query/queryClient.ts', import.meta.url).pathname)}`,
          `export { keys } from ${JSON.stringify(new URL('./query/keys.ts', import.meta.url).pathname)}`,
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

afterEach(async () => {
  if (activeModule) {
    await activeModule.signOutCloudAccount().catch(() => {})
    activeModule.disconnectBackend()
  }
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
  localEnabled = true,
  cloudUrl = '',
  cloudStatusResponse = { active_workspace_id: 'local', connected: false, status: 'disconnected', workspaces: [] },
  onLocalCloudConnect,
  onLocalCloudSwitch,
  onLocalCloudDisconnect,
  onLocalCloudSync,
  onCloudAuthMe,
  orgIdentity,
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
      cloudUrl,
      localEnabled,
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
      if (target.pathname === '/v1/cloud/switch' && onLocalCloudSwitch) {
        return onLocalCloudSwitch(init)
      }
      if (target.pathname === '/v1/cloud/disconnect') {
        return onLocalCloudDisconnect
          ? onLocalCloudDisconnect(init)
          : Response.json({ active_workspace_id: 'local', connected: false, status: 'disconnected', workspaces: [] })
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
    if (target.pathname === '/v1/auth/me' && onCloudAuthMe) {
      return onCloudAuthMe(init)
    }
    if (target.pathname === '/v1/auth/logout') {
      return new Response(null, { status: 204 })
    }
    throw new Error(`Desktop should never send API requests directly to ${url}`)
  })

  const module = { exports: {} }
  evaluate(
    (name) => {
      if (name === 'react') {
        return { ...React, useSyncExternalStore: (_sub, snap) => snap(), useEffect: (effect) => effect() }
      }
      if (name === '@tanstack/react-query') {
        return { ...require(name), useQuery: (options) => ({ data: options.enabled ? orgIdentity : undefined }) }
      }
      return require(name)
    },
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
  const acmeWorkspace = {
    id: 'ws_acme',
    url: cloudUrl,
    org_id: 'org_acme',
    org_name: 'Acme Cloud',
    email: 'member@acme.example',
    display_name: 'Alex',
    role: 'admin',
    last_sync_at: '2026-11-22T18:00:00Z',
  }
  let connectPayload = null
  let disconnectCalls = 0
  let syncCalls = 0

  const f = await fixture({
    onLocalCloudConnect: (init) => {
      connectPayload = JSON.parse(String(init.body))
      return Response.json({
        active_workspace_id: 'ws_acme',
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
        workspaces: [acmeWorkspace],
      })
    },
    onLocalCloudSync: () => {
      syncCalls += 1
      return Response.json({
        active_workspace_id: 'ws_acme',
        connected: true,
        status: 'connected',
        url: cloudUrl,
        last_sync_at: '2026-11-22T18:05:00Z',
        user: { id: 'usr_1', org_id: 'org_acme', org_name: 'Acme Cloud', email: 'member@acme.example', display_name: 'Alex', role: 'admin' },
        workspaces: [{ ...acmeWorkspace, last_sync_at: '2026-11-22T18:05:00Z' }],
      })
    },
    onLocalCloudDisconnect: () => {
      disconnectCalls += 1
      return Response.json({ active_workspace_id: 'local', connected: false, status: 'disconnected', workspaces: [] })
    },
  })

  expect(f.mod.useConnection()).toMatchObject({ status: 'connected', url: 'http://127.0.0.1:5299', mode: 'local' })
  expect(f.mod.currentCloudAccount()).toMatchObject({ status: 'disconnected', activeWorkspaceId: 'local', workspaces: [] })

  expect(await f.mod.attachCloudSignIn(cloudUrl)).toBeNull()
  expect(f.oidcBeginCalls()).toBe(1)
  const loginUrl = new URL(f.openedExternal[0])
  const state = loginUrl.searchParams.get('state')
  const pending = JSON.parse(f.sessionStored.get('jaz.oidc.pending'))

  f.triggerOidcCallback(`jaz://auth/callback#code=attach-code-1&state=${state}`)
  await settle()

  expect(connectPayload).toEqual({
    url: cloudUrl,
    code: 'attach-code-1',
    code_verifier: pending.codeVerifier,
  })
  expect([...f.stored.keys()].filter((k) => k.includes('cloudAccount'))).toEqual([])
  expect(f.mod.currentCloudAccount()).toMatchObject({
    status: 'connected',
    activeWorkspaceId: 'ws_acme',
    url: cloudUrl,
    lastSyncAt: '2026-11-22T18:00:00Z',
    user: { id: 'usr_1', org_name: 'Acme Cloud' },
  })
  expect(f.mod.currentCloudAccount().workspaces).toEqual([acmeWorkspace])

  expect(await f.mod.syncCloudAccount()).toBeNull()
  expect(syncCalls).toBe(1)
  expect(f.mod.currentCloudAccount().lastSyncAt).toBe('2026-11-22T18:05:00Z')

  await f.mod.signOutCloudAccount()
  expect(disconnectCalls).toBe(1)
  expect(f.mod.currentCloudAccount()).toMatchObject({
    status: 'disconnected',
    activeWorkspaceId: 'local',
    workspaces: [],
  })
  expect(f.mod.useConnection()).toMatchObject({ status: 'connected', mode: 'local' })
})

test('Multi-workspace status, switchWorkspace, and per-workspace signOutCloudAccount(id) keep other Cloud workspaces intact', async () => {
  const wsAcme = {
    id: 'ws_acme',
    url: 'https://acme.jaz.example',
    org_id: 'org_acme',
    org_name: 'Acme Cloud',
    email: 'dev@acme.example',
    display_name: 'Dev',
    role: 'admin',
  }
  const wsPersonal = {
    id: 'ws_personal',
    url: 'https://personal.jaz.example',
    org_id: 'org_personal',
    org_name: 'Personal Cloud',
    email: 'me@personal.example',
    display_name: 'Me',
    role: 'member',
  }

  let workspaces = [wsAcme, wsPersonal]
  let activeId = 'ws_acme'
  const switchPayloads = []
  const disconnectPayloads = []

  const buildStatus = () => {
    const active = workspaces.find((ws) => ws.id === activeId)
    if (!active) {
      return {
        active_workspace_id: 'local',
        connected: false,
        status: 'disconnected',
        workspaces,
      }
    }
    return {
      active_workspace_id: active.id,
      connected: true,
      status: 'connected',
      url: active.url,
      user: {
        id: `usr_${active.id}`,
        org_id: active.org_id,
        org_name: active.org_name,
        email: active.email,
        display_name: active.display_name,
        role: active.role,
      },
      workspaces,
    }
  }

  const f = await fixture({
    cloudStatusResponse: buildStatus(),
    onLocalCloudSwitch: (init) => {
      const body = JSON.parse(String(init.body))
      switchPayloads.push(body)
      activeId = body.workspace_id
      return Response.json(buildStatus())
    },
    onLocalCloudDisconnect: (init) => {
      const body = init.body ? JSON.parse(String(init.body)) : {}
      disconnectPayloads.push(body)
      const targetId = body.workspace_id || activeId
      workspaces = workspaces.filter((ws) => ws.id !== targetId)
      if (activeId === targetId) {
        activeId = workspaces[0]?.id ?? 'local'
      }
      return Response.json(buildStatus())
    },
  })

  expect(f.mod.currentCloudAccount()).toMatchObject({
    status: 'connected',
    activeWorkspaceId: 'ws_acme',
    url: 'https://acme.jaz.example',
  })
  expect(f.mod.currentCloudAccount().workspaces).toHaveLength(2)
  expect(f.mod.useCloudSession()).toMatchObject({
    active: true,
    activeWorkspaceId: 'ws_acme',
    workspaces: [wsAcme, wsPersonal],
  })

  const { queryClient, keys } = f.mod
  const seedWorkspaceCaches = () => {
    queryClient.setQueryData(keys.sidebarSessions, [{ id: 'thr_1' }])
    queryClient.setQueryData([...keys.archivedSessions, ''], { pages: [{ sessions: [{ id: 'thr_arch' }] }] })
    queryClient.setQueryData(keys.session('thr_1'), { id: 'thr_1' })
    queryClient.setQueryData(keys.usageDaily({ days: 30 }, 'UTC'), [])
    queryClient.setQueryData(keys.agentFiles, { files: [] })
    queryClient.setQueryData(keys.projects, [{ path: '/repo' }])
    queryClient.setQueryData(keys.bots, [{ id: 'bot_1' }])
    queryClient.setQueryData(keys.botActivity('bot_1'), { items: [], owed: [] })
    queryClient.setQueryData(keys.boards, [{ id: 'brd_1' }])
    queryClient.setQueryData(keys.boardDetail('brd_1'), { board: { id: 'brd_1' }, items: [] })
    queryClient.setQueryData(keys.loops, [{ id: 'lp_1' }])
    queryClient.setQueryData(keys.loopDetail('lp_1'), { loop: { id: 'lp_1' }, runs: [], boardIds: [] })
    queryClient.setQueryData(keys.botRoutines('bot_1'), [])
    queryClient.setQueryData(keys.plugins, { plugins: [], sources: [] })
    queryClient.setQueryData(keys.pluginPage('tenders'), { view: '', items: [] })
    queryClient.setQueryData(keys.mcpServers, { servers: [], oauth_redirect_url: '' })
    queryClient.setQueryData(keys.mcpApps, [])
    queryClient.setQueryData(keys.mcpApp('crm', 'show'), '<html/>')
    queryClient.setQueryData(keys.feed, [{ id: 'feed_1' }])
    queryClient.setQueryData(keys.threadSearch('demo', true), [{ thread_id: 'thr_1' }])
    queryClient.setQueryData(keys.connectionSearch('demo'), [{ server_id: 'crm' }])
    queryClient.setQueryData(keys.skills('/repo'), [{ name: 'skill-a' }])
    queryClient.setQueryData(keys.cloudSpaces, [{ id: 'spc_1' }])
    queryClient.setQueryData(keys.cloudMachines, [{ id: 'mac_1' }])
    queryClient.setQueryData(keys.cloudUsageSummary, { user_spent_usd: 1 })
    queryClient.setQueryData(keys.cloudInstruction('user', 'u1', 'agents'), { body_md: 'hi' })
    // Device-global caches must remain intact across workspace switches.
    queryClient.setQueryData(keys.agentSettings, { default_agent: 'codex' })
    queryClient.setQueryData(keys.browserSettings, { enabled: true })
  }
  seedWorkspaceCaches()

  const workspaceChanges = []
  const unsubscribeChanges = f.mod.onWorkspaceChange((id) => workspaceChanges.push(id))

  const invalidated = []
  const origInvalidate = queryClient.invalidateQueries.bind(queryClient)
  queryClient.invalidateQueries = (opts) => {
    invalidated.push(opts?.queryKey)
    return origInvalidate(opts)
  }

  // Initial hydration must not have emitted a spurious workspace-switch event.
  expect(workspaceChanges).toEqual([])

  // Switch to Local workspace: Cloud becomes inactive while preserving both Cloud workspaces.
  expect(await f.mod.switchWorkspace('local')).toBeNull()
  expect(switchPayloads).toEqual([{ workspace_id: 'local' }])
  expect(workspaceChanges).toEqual(['local'])
  expect(f.mod.currentCloudAccount()).toMatchObject({
    status: 'disconnected',
    activeWorkspaceId: 'local',
    user: null,
  })
  expect(f.mod.currentCloudAccount().workspaces).toEqual([wsAcme, wsPersonal])
  expect(f.mod.useCloudSession().active).toBe(false)
  expect(f.mod.useHasCloud()).toBe(false)

  // Every workspace-scoped query root is invalidated and stale cached data is evicted.
  for (const expectedKey of [
    keys.sessions,
    keys.usage,
    keys.agentFiles,
    keys.projects,
    keys.bots,
    keys.boards,
    keys.loops,
    keys.plugins,
    keys.mcp,
    keys.feed,
    keys.search,
    keys.skillsRoot,
  ]) {
    expect(invalidated).toContainEqual(expectedKey)
  }
  expect(queryClient.getQueryData(keys.sidebarSessions)).toBeUndefined()
  expect(queryClient.getQueryData([...keys.archivedSessions, ''])).toBeUndefined()
  expect(queryClient.getQueryData(keys.session('thr_1'))).toBeUndefined()
  expect(queryClient.getQueryData(keys.bots)).toBeUndefined()
  expect(queryClient.getQueryData(keys.botActivity('bot_1'))).toBeUndefined()
  expect(queryClient.getQueryData(keys.boards)).toBeUndefined()
  expect(queryClient.getQueryData(keys.boardDetail('brd_1'))).toBeUndefined()
  expect(queryClient.getQueryData(keys.loops)).toBeUndefined()
  expect(queryClient.getQueryData(keys.loopDetail('lp_1'))).toBeUndefined()
  expect(queryClient.getQueryData(keys.botRoutines('bot_1'))).toBeUndefined()
  expect(queryClient.getQueryData(keys.plugins)).toBeUndefined()
  expect(queryClient.getQueryData(keys.pluginPage('tenders'))).toBeUndefined()
  expect(queryClient.getQueryData(keys.mcpServers)).toBeUndefined()
  expect(queryClient.getQueryData(keys.mcpApps)).toBeUndefined()
  expect(queryClient.getQueryData(keys.mcpApp('crm', 'show'))).toBeUndefined()
  expect(queryClient.getQueryData(keys.feed)).toBeUndefined()
  expect(queryClient.getQueryData(keys.threadSearch('demo', true))).toBeUndefined()
  expect(queryClient.getQueryData(keys.connectionSearch('demo'))).toBeUndefined()
  expect(queryClient.getQueryData(keys.skills('/repo'))).toBeUndefined()
  expect(queryClient.getQueryData(keys.cloudSpaces)).toBeUndefined()
  expect(queryClient.getQueryData(keys.cloudMachines)).toBeUndefined()
  expect(queryClient.getQueryData(keys.cloudUsageSummary)).toBeUndefined()
  expect(queryClient.getQueryData(keys.cloudInstruction('user', 'u1', 'agents'))).toBeUndefined()
  // Device-global queries stay cached.
  expect(queryClient.getQueryData(keys.agentSettings)).toEqual({ default_agent: 'codex' })
  expect(queryClient.getQueryData(keys.browserSettings)).toEqual({ enabled: true })

  // Switch to second Cloud workspace: cloud root is also invalidated for refetch.
  invalidated.length = 0
  seedWorkspaceCaches()
  expect(await f.mod.switchWorkspace('ws_personal')).toBeNull()
  expect(f.mod.currentCloudAccount()).toMatchObject({
    status: 'connected',
    activeWorkspaceId: 'ws_personal',
    url: 'https://personal.jaz.example',
    user: { org_name: 'Personal Cloud' },
  })
  expect(f.mod.useHasCloud()).toBe(true)
  expect(invalidated).toContainEqual(keys.cloud)
  expect(invalidated).toContainEqual(keys.bots)
  expect(invalidated).toContainEqual(keys.boards)
  expect(invalidated).toContainEqual(keys.loops)
  expect(invalidated).toContainEqual(keys.plugins)
  expect(invalidated).toContainEqual(keys.mcp)
  expect(queryClient.getQueryData(keys.bots)).toBeUndefined()
  expect(queryClient.getQueryData(keys.boards)).toBeUndefined()

  // Sign out inactive ws_acme: keeps ws_personal active and leaves its cached data intact.
  queryClient.setQueryData(keys.bots, [{ id: 'bot_personal' }])
  await f.mod.signOutCloudAccount('ws_acme')
  expect(disconnectPayloads).toEqual([{ workspace_id: 'ws_acme' }])
  expect(f.mod.currentCloudAccount()).toMatchObject({
    status: 'connected',
    activeWorkspaceId: 'ws_personal',
    url: 'https://personal.jaz.example',
  })
  expect(f.mod.currentCloudAccount().workspaces).toEqual([wsPersonal])
  expect(queryClient.getQueryData(keys.bots)).toEqual([{ id: 'bot_personal' }])
  expect(workspaceChanges).toEqual(['local', 'ws_personal'])

  // Sign out active ws_personal: falls back to local and clears workspace-scoped caches.
  await f.mod.signOutCloudAccount('ws_personal')
  expect(f.mod.currentCloudAccount()).toMatchObject({
    status: 'disconnected',
    activeWorkspaceId: 'local',
    workspaces: [],
  })
  expect(queryClient.getQueryData(keys.bots)).toBeUndefined()
  expect(workspaceChanges).toEqual(['local', 'ws_personal', 'local'])
  unsubscribeChanges()
})

test('Cloud-only primary Cloud mode exposes connected primary workspace and NEVER activeWorkspaceId === local', async () => {
  const cloudUrl = 'https://org.jaz.example'
  const f = await fixture({
    localEnabled: false,
    cloudUrl,
    orgIdentity: { org_id: 'org_cas', org_name: 'CAS Renamed', org_icon: { kind: 'preset', preset: 'rocket' } },
    onCloudAuthMe: (init) => {
      if (new Headers(init?.headers).get('Authorization') !== 'Bearer cloud-tok') {
        return Response.json({ error: 'authentication required' }, { status: 401 })
      }
      return Response.json({
        id: 'usr_cas',
        org_id: 'org_cas',
        org_name: 'Cambridge Advanced Systems',
        email: 'eng@cas.example',
        display_name: 'Engineer',
        role: 'admin',
      })
    },
  })

  // Disconnected in cloud-only mode: still never 'local'.
  expect(f.mod.useCloudSession()).toMatchObject({
    active: false,
    source: 'primary',
    activeWorkspaceId: '',
    workspaces: [],
  })
  expect(f.mod.useCloudSession().activeWorkspaceId).not.toBe('local')

  // Connecting to local loopback is blocked in cloud-only builds.
  expect(await f.mod.connectRemote('http://127.0.0.1:5299')).toContain('disabled')

  // Connecting to primary Cloud populates workspaces and activeWorkspaceId.
  expect(await f.mod.connectRemote(`${cloudUrl}#token=cloud-tok`)).toBeNull()
  expect(f.mod.useCloudSession()).toMatchObject({
    active: true,
    source: 'primary',
    activeWorkspaceId: 'org_cas',
    workspaces: [
      {
        id: 'org_cas',
        url: cloudUrl,
        org_id: 'org_cas',
        org_name: 'CAS Renamed',
        org_icon: { kind: 'preset', preset: 'rocket' },
        email: 'eng@cas.example',
        role: 'admin',
      },
    ],
  })
  expect(f.mod.useCloudSession().activeWorkspaceId).not.toBe('local')
})
