import { afterEach, expect, test } from 'bun:test'
import { build } from 'bun'
import { createRequire } from 'node:module'

const bundle = await build({
  entrypoints: ['cloud-api-fixture'],
  target: 'browser',
  format: 'cjs',
  define: { 'import.meta.env': '{}' },
  plugins: [{
    name: 'cloud-api-fixture',
    setup(builder) {
      builder.onResolve({ filter: /^cloud-api-fixture$/ }, () => ({ path: 'entry', namespace: 'fixture' }))
      builder.onResolve({ filter: /^\//, namespace: 'fixture' }, (args) => ({ path: args.path, namespace: 'file' }))
      builder.onLoad({ filter: /.*/, namespace: 'fixture' }, () => ({
        contents: [
          `export * from ${JSON.stringify(new URL('./cloud.ts', import.meta.url).pathname)}`,
          `export * from ${JSON.stringify(new URL('./client.ts', import.meta.url).pathname)}`,
          `export * from ${JSON.stringify(new URL('../connectionProbe.ts', import.meta.url).pathname)}`,
          `export * from ${JSON.stringify(new URL('../backends.ts', import.meta.url).pathname)}`,
          `export * from ${JSON.stringify(new URL('../modelPicker.ts', import.meta.url).pathname)}`,
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

function replace(name, value) {
  if (!originals.has(name)) {
    originals.set(name, Object.getOwnPropertyDescriptor(globalThis, name))
  }
  Object.defineProperty(globalThis, name, { configurable: true, writable: true, value })
}

afterEach(() => {
  for (const [name, descriptor] of originals) {
    if (descriptor) {
      Object.defineProperty(globalThis, name, descriptor)
    } else {
      delete globalThis[name]
    }
  }
  originals.clear()
})

function fixture({ web = false, cloudUrl = '', locationHref = 'https://app.jaz.chat/', storedValues = [] } = {}) {
  const stored = new Map(storedValues)
  const sessionStored = new Map()
  const historyUpdates = []
  const loc = new URL(locationHref)
  replace('window', {
    location: {
      href: loc.href,
      origin: loc.origin,
      pathname: loc.pathname,
      search: loc.search,
      hash: loc.hash,
    },
    history: {
      state: null,
      replaceState(_state, _title, next) {
        historyUpdates.push(String(next))
      },
    },
    addEventListener() {},
    jaz: web ? undefined : {
      apiBaseUrl: 'http://127.0.0.1:5299',
      cloudUrl,
      localEnabled: true,
      windowKind: 'main',
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

  const module = { exports: {} }
  evaluate(require, module, module.exports)
  const api = module.exports
  api.setApiBaseUrl('https://cloud.jaz.example')
  api.setApiAuthToken('https://cloud.jaz.example', 'oidc-session-token')
  return { api, stored, sessionStored, historyUpdates }
}

test('fetchServerDiscovery parses Cloud OIDC discovery payload', async () => {
  const { api } = fixture()
  replace('fetch', async (url) => {
    expect(String(url)).toBe('https://jaz-cloud.example/.well-known/jaz-server')
    return Response.json({
      ok: true,
      mode: 'cloud',
      auth: 'oidc',
      oidc_login_url: '/v1/auth/login',
      oidc_issuer: 'https://login.example.com',
      public_url: 'https://jaz-cloud.example',
      capabilities: {
        spaces: true,
        cloud_instructions: true,
        connected_machines: true,
        native_codex_exec_remote: true,
      },
    })
  })
  const discovered = await api.fetchServerDiscovery('https://jaz-cloud.example/')
  expect(discovered).toEqual({
    ok: true,
    mode: 'cloud',
    auth: 'oidc',
    authRequired: true,
    oidcLoginUrl: '/v1/auth/login',
    oidcIssuer: 'https://login.example.com',
    publicUrl: 'https://jaz-cloud.example',
    capabilities: {
      spaces: true,
      cloud_instructions: true,
      connected_machines: true,
      native_codex_exec_remote: true,
    },
  })
})

test('fetchServerDiscovery parses Local bearer discovery and falls back to /health on 404', async () => {
  const { api } = fixture()
  replace('fetch', async (url) => {
    expect(String(url)).toBe('http://127.0.0.1:5299/.well-known/jaz-server')
    return Response.json({ ok: true, mode: 'local', auth: 'bearer', capabilities: { session_file_read: true } })
  })
  const localDirect = await api.fetchServerDiscovery('http://127.0.0.1:5299')
  expect(localDirect).toMatchObject({ ok: true, mode: 'local', auth: 'bearer', authRequired: true })

  const requested = []
  replace('fetch', async (url) => {
    requested.push(new URL(String(url)).pathname)
    if (String(url).endsWith('/.well-known/jaz-server')) {
      return new Response('not found', { status: 404 })
    }
    return Response.json({ ok: true, auth_required: false, capabilities: { session_file_read: true } })
  })
  const legacy = await api.fetchServerDiscovery('http://127.0.0.1:5299')
  expect(requested).toEqual(['/.well-known/jaz-server', '/health'])
  expect(legacy).toMatchObject({ ok: true, mode: 'local', auth: 'none', authRequired: false })

  const spa = fixture({ web: true })
  replace('fetch', async (url) => {
    if (String(url).endsWith('/.well-known/jaz-server')) {
      return new Response('<!doctype html><html></html>', {
        status: 200,
        headers: { 'content-type': 'text/html' },
      })
    }
    return Response.json({ ok: true, auth_required: true })
  })
  expect(await spa.api.fetchServerDiscovery('https://legacy.example')).toMatchObject({
    ok: true,
    mode: 'local',
    auth: 'bearer',
    authRequired: true,
  })
})

test('fetchServerDiscovery rejects non-HTTPS remote URLs without network calls', async () => {
  const { api } = fixture()
  let hits = 0
  replace('fetch', async () => {
    hits += 1
    return Response.json({ ok: true })
  })
  const result = await api.fetchServerDiscovery('http://insecure.example')
  expect(result).toEqual({ ok: false, error: 'Remote connections require HTTPS', recoverable: false })
  expect(hits).toBe(0)
})

test('OIDC URL builder, #token= hash parser, and startup callback consumption', () => {
  const { api, historyUpdates } = fixture({
    web: true,
    locationHref: 'https://app.jaz.chat/login#token=oidc_tok_123',
  })
  expect(api.buildOidcLoginUrl('https://cloud.jaz.example/', '/v1/auth/login')).toBe(
    'https://cloud.jaz.example/v1/auth/login',
  )
  expect(
    api.buildOidcLoginUrl('https://cloud.jaz.example', '/v1/auth/login', 'jaz://auth/callback'),
  ).toBe('https://cloud.jaz.example/v1/auth/login?client_redirect=jaz%3A%2F%2Fauth%2Fcallback')

  expect(api.parseOidcCallbackHash('#token=abc123&server=https://cloud.jaz.example')).toEqual({
    token: 'abc123',
    server: 'https://cloud.jaz.example',
  })
  expect(api.parseOidcCallbackHash('https://app.jaz.chat/#token=xyz789')).toEqual({
    token: 'xyz789',
    server: '',
  })

  api.rememberPendingOidcServer('https://cloud.jaz.example')
  const consumed = api.consumeStartupConnectUrl()
  expect(consumed).toBe('https://cloud.jaz.example/#key=oidc_tok_123')
  expect(historyUpdates).toEqual(['https://app.jaz.chat/login'])
})

test('Spaces, Thread User State, Instructions 409 conflict, and Connected Machines API calls', async () => {
  const { api } = fixture()
  const calls = []
  replace('fetch', async (url, init) => {
    const parsed = new URL(String(url))
    const body = init?.body ? JSON.parse(String(init.body)) : undefined
    calls.push({ method: init?.method || 'GET', path: parsed.pathname, body })

    if (parsed.pathname === '/v1/auth/me') {
      return Response.json({
        user_id: 'usr_1',
        org_id: 'org_acme',
        org_name: 'Acme',
        email: 'member@acme.example',
        display_name: 'Alex',
        role: 'member',
      })
    }
    if (parsed.pathname === '/v1/spaces' && (init?.method || 'GET') === 'GET') {
      return Response.json({ spaces: [{ id: 'spc_1', name: 'Engineering', member_count: 4 }] })
    }
    if (parsed.pathname === '/v1/spaces' && init?.method === 'POST') {
      return Response.json({ id: 'spc_2', name: body.name })
    }
    if (parsed.pathname === '/v1/spaces/spc_2/members' && init?.method === 'POST') {
      return Response.json({ space_id: 'spc_2', user_id: 'usr_2', email: body.email, role: body.role })
    }
    if (parsed.pathname === '/v1/threads/thr_1/user-state' && init?.method === 'PUT') {
      return Response.json({ thread_id: 'thr_1', personal_sources_enabled: body.personal_sources_enabled })
    }
    if (parsed.pathname === '/v1/instructions/org/org_acme/agents' && (init?.method || 'GET') === 'GET') {
      return Response.json({
        key: { scope: 'org', scope_id: 'org_acme', kind: 'agents' },
        content: '# Org Rules',
        version: 'abc1234567890def',
      })
    }
    if (parsed.pathname === '/v1/instructions/org/org_acme/agents' && init?.method === 'PUT') {
      return Response.json({ error: 'the file changed since it was read' }, { status: 409 })
    }
    if (parsed.pathname === '/v1/machines') {
      return Response.json({
        machines: [{
          device_id: 'mac-work',
          display_name: 'Work MacBook',
          platform: 'darwin',
          online: true,
          capabilities: { file_staging: true, local_exec: true },
          folder_grants: [{ id: 'repo', root_path: '/Users/dev/acme', mode: 'rw' }],
          last_seen_at: '2026-11-22T12:00:00Z',
        }],
      })
    }
    throw new Error(`Unexpected ${init?.method || 'GET'} ${parsed.pathname}`)
  })

  const me = await api.fetchAuthMe()
  expect(me).toEqual({
    id: 'usr_1',
    org_id: 'org_acme',
    org_name: 'Acme',
    email: 'member@acme.example',
    display_name: 'Alex',
    role: 'member',
  })

  expect(await api.listSpaces()).toEqual([{ id: 'spc_1', name: 'Engineering', member_count: 4 }])
  expect(await api.createSpace({ name: 'Platform' })).toEqual({ id: 'spc_2', name: 'Platform' })
  expect(await api.addSpaceMember('spc_2', { email: 'teammate@acme.example', role: 'admin' })).toEqual({
    space_id: 'spc_2',
    user_id: 'usr_2',
    email: 'teammate@acme.example',
    role: 'admin',
  })
  expect(await api.updateThreadUserState('thr_1', { personal_sources_enabled: true })).toEqual({
    thread_id: 'thr_1',
    personal_sources_enabled: true,
  })

  const doc = await api.getInstruction('org', 'org_acme', 'agents')
  expect(doc.content).toBe('# Org Rules')
  expect(doc.version).toBe('abc1234567890def')

  let conflictError
  try {
    await api.saveInstruction('org', 'org_acme', 'agents', { content: '# Updated', expected_version: doc.version })
  } catch (error) {
    conflictError = error
  }
  expect(conflictError).toBeInstanceOf(api.ApiError)
  expect(conflictError.status).toBe(409)

  const machines = await api.listMachines()
  expect(machines).toHaveLength(1)
  expect(machines[0].folder_grants).toEqual([{ id: 'repo', root_path: '/Users/dev/acme', mode: 'rw' }])

  const cmd = api.buildConnectCloudCommand({
    cloudUrl: 'https://cloud.jaz.example/',
    deviceId: 'mac-1',
    displayName: 'Studio Mac',
    grants: [{ id: 'proj', root_path: '/Users/dev/My Repo', mode: 'rw' }],
    allowExec: true,
  })
  expect(cmd).toContain('--url https://cloud.jaz.example')
  expect(cmd).toContain("--name 'Studio Mac'")
  expect(cmd).toContain("--grant 'proj=rw:/Users/dev/My Repo'")
  expect(cmd).toContain('--allow-exec')
})

test('uploadCloudFile executes presign -> direct PUT without Jaz auth -> commit with SHA-256', async () => {
  const { api } = fixture()
  const steps = []
  const progress = []
  replace('fetch', async (url, init) => {
    const rawUrl = String(url)
    if (rawUrl.endsWith('/v1/runs/run_42/uploads/presign')) {
      const body = JSON.parse(String(init.body))
      steps.push({ step: 'presign', body, auth: new Headers(init.headers).get('Authorization') })
      return Response.json({
        upload_url: 'https://storage.googleapis.com/jaz-bucket/signed-put?sig=1',
        headers: { 'x-goog-content-length-range': '0,1048576' },
        object_key: 'org_acme/u1/run_42/objects/deadbeef',
        input_id: 'inp_99',
        relpath: body.relpath,
      })
    }
    if (rawUrl.startsWith('https://storage.googleapis.com/')) {
      const headers = new Headers(init.headers)
      steps.push({
        step: 'put',
        method: init.method,
        auth: headers.get('Authorization'),
        contentType: headers.get('Content-Type'),
        googHeader: headers.get('x-goog-content-length-range'),
      })
      return new Response(null, { status: 200 })
    }
    if (rawUrl.endsWith('/v1/runs/run_42/uploads/commit')) {
      const body = JSON.parse(String(init.body))
      steps.push({ step: 'commit', body })
      return Response.json({
        upload_seq: 1,
        input_id: body.input_id,
        relpath: body.relpath,
        object_key: body.object_key,
        sha256: body.sha256,
        size_bytes: body.size_bytes,
        content_type: body.content_type,
      })
    }
    throw new Error(`Unexpected request ${rawUrl}`)
  })

  const file = new File(['hello cloud'], 'notes.txt', { type: 'text/plain' })
  const attachment = await api.uploadCloudFile('run_42', file, { onProgress: (p) => progress.push(p) })
  expect(steps.map((s) => s.step)).toEqual(['presign', 'put', 'commit'])
  expect(steps[0].auth).toBe('Bearer oidc-session-token')
  expect(steps[0].body.sha256).toHaveLength(64)
  expect(steps[1]).toEqual({
    step: 'put',
    method: 'PUT',
    auth: null,
    contentType: file.type,
    googHeader: '0,1048576',
  })
  expect(steps[2].body.sha256).toBe(steps[0].body.sha256)
  expect(steps[2].body.input_id).toBe('inp_99')
  expect(attachment).toEqual({
    id: 'inp_99',
    name: 'notes.txt',
    uri: 'https://cloud.jaz.example/v1/runs/run_42/files/notes.txt',
    mime_type: file.type,
    size: 11,
  })
  expect(progress.at(-1)).toBe(1)
})

test('evaluateSpendStatus and isSpendCapError handle warning, cap reached, and HTTP 402', async () => {
  const { api } = fixture()
  expect(api.evaluateSpendStatus({ org_spend_usd: 20, org_cap_usd: 100 })).toBeNull()
  expect(api.evaluateSpendStatus({ org_spend_usd: 85, org_cap_usd: 100 })).toMatchObject({
    level: 'warning',
    scope: 'org',
    percent: 85,
  })
  expect(api.evaluateSpendStatus({ org_spend_usd: 50, org_cap_usd: 200, user_spend_usd: 25, user_cap_usd: 25 })).toMatchObject({
    level: 'exceeded',
    scope: 'user',
    percent: 100,
  })
  expect(api.isSpendCapError(new api.ApiError(402, 'Monthly spend cap exceeded'))).toBe(true)
  expect(api.isSpendCapError(new api.ApiError(500, 'Internal error'))).toBe(false)
})

test('renameBackend derives managed from cloudUrl without persisting managed: true to localStorage', () => {
  const { api, stored } = fixture({ cloudUrl: 'https://cloud.acme.example' })
  api.renameBackend('https://cloud.acme.example', 'Acme Cloud')
  const persisted = JSON.parse(stored.get('jaz.backends') || '[]')
  expect(persisted).toHaveLength(1)
  expect(persisted[0].label).toBe('Acme Cloud')
  expect('managed' in persisted[0]).toBe(false)
  expect(api.knownBackend('https://cloud.acme.example')?.managed).toBe(true)
})

test('cloud model profiles map OpenAI, Anthropic, and xAI into grouped Cloud vs Local picker options', async () => {
  const { api } = fixture()
  replace('fetch', async (url) => {
    expect(new URL(String(url)).pathname).toBe('/v1/models')
    return Response.json({
      model_profiles: [
        { profile_id: 'openai-prod', provider: 'openai', default_model: 'gpt-6.1-sol', default_effort: 'high' },
        { profile_id: 'anthropic-prod', provider: 'anthropic', default_model: 'claude-opus-5.5' },
        { profile_id: 'xai-prod', provider: 'xai', default_model: 'grok-4' },
      ],
    })
  })
  const profiles = await api.listCloudModelProfiles()
  const cloudItems = profiles.map(api.cloudProfileToAgentOption)
  expect(cloudItems).toEqual([
    { id: 'openai-prod', label: 'Codex', logoAgent: 'codex', location: 'cloud', provider: 'openai' },
    { id: 'anthropic-prod', label: 'Claude', logoAgent: 'claude', location: 'cloud', provider: 'anthropic' },
    { id: 'xai-prod', label: 'Grok', logoAgent: 'grok', location: 'cloud', provider: 'xai' },
  ])
  const grouped = api.groupPickerAgents([
    ...cloudItems,
    { id: 'codex', location: 'local' },
  ])
  expect(grouped.map((group) => [group.location, group.label, group.items.length])).toEqual([
    ['cloud', 'Cloud', 3],
    ['local', 'Local', 1],
  ])
})
