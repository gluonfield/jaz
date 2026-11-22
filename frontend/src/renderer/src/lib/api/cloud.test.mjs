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
          `export * from ${JSON.stringify(new URL('./serverTypes.ts', import.meta.url).pathname)}`,
          `export * from ${JSON.stringify(new URL('./cloudAuth.ts', import.meta.url).pathname)}`,
          `export * from ${JSON.stringify(new URL('./cloudSpaces.ts', import.meta.url).pathname)}`,
          `export * from ${JSON.stringify(new URL('./cloudInstructions.ts', import.meta.url).pathname)}`,
          `export * from ${JSON.stringify(new URL('./cloudUploads.ts', import.meta.url).pathname)}`,
          `export * from ${JSON.stringify(new URL('./cloudUsage.ts', import.meta.url).pathname)}`,
          `export * from ${JSON.stringify(new URL('./cloudThreads.ts', import.meta.url).pathname)}`,
          `export * from ${JSON.stringify(new URL('./sessions.ts', import.meta.url).pathname)}`,
          `export * from ${JSON.stringify(new URL('./sse.ts', import.meta.url).pathname)}`,
          `export * from ${JSON.stringify(new URL('./client.ts', import.meta.url).pathname)}`,
          `export * from ${JSON.stringify(new URL('./cloudClient.ts', import.meta.url).pathname)}`,
          `export * from ${JSON.stringify(new URL('../sessionPrompt.ts', import.meta.url).pathname)}`,
          `export * from ${JSON.stringify(new URL('../cloudMachineConfig.ts', import.meta.url).pathname)}`,
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
      replaceState() {},
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
  api.setApiServerMode('cloud')
  api.setApiAuthToken('https://cloud.jaz.example', 'oidc-session-token', {
    token: 'rt_signed_abc',
    expires_at: '2099-01-01T00:00:00Z',
  })
  return { api, stored, sessionStored }
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

test('OIDC URL builder enforces same-origin HTTPS, callback parser ignores query params, and state + single-use exchange work', async () => {
  const { api } = fixture({ web: true })
  expect(api.buildOidcLoginUrl('https://cloud.jaz.example/', '/v1/auth/login')).toBe(
    'https://cloud.jaz.example/v1/auth/login',
  )
  expect(
    api.buildOidcLoginUrl('https://cloud.jaz.example', '/v1/auth/login', {
      clientRedirect: 'jaz://auth/callback',
      state: 'st_123',
    }),
  ).toBe('https://cloud.jaz.example/v1/auth/login?client_redirect=jaz%3A%2F%2Fauth%2Fcallback&state=st_123')

  expect(() => api.buildOidcLoginUrl('https://cloud.jaz.example', 'https://evil.example/phish')).toThrow('origin')
  expect(() => api.buildOidcLoginUrl('https://cloud.jaz.example', '//evil.example/phish')).toThrow('origin')
  expect(() => api.buildOidcLoginUrl('https://cloud.jaz.example', '/\\evil.example/phish')).toThrow('origin')
  expect(() => api.buildOidcLoginUrl('http://cloud.jaz.example', '/v1/auth/login')).toThrow('HTTPS')

  // Query-string credentials/codes are never parsed.
  expect(api.parseOidcCallbackUrl('https://app.jaz.chat/?token=leaked&code=leaked&state=s1')).toEqual({
    code: '',
    token: '',
    state: '',
    server: '',
  })
  expect(api.parseOidcCallbackUrl('jaz://auth/callback#code=one_time&state=st_123')).toEqual({
    code: 'one_time',
    token: '',
    state: 'st_123',
    server: '',
  })

  // Unsolicited callback without pending sessionStorage state fails.
  expect(api.validateOidcCallback('#code=one_time&state=st_123', null).ok).toBe(false)

  // Single-use pending state validates exact server & state and rejects mismatched state/server or replay.
  const pending = api.beginPendingOidcLogin('https://cloud.jaz.example', () => 10_000)
  expect(api.validateOidcCallback(`#code=one_time&state=wrong`, pending, () => 15_000).ok).toBe(false)
  expect(api.validateOidcCallback(`#token=raw_tok`, pending, () => 15_000).ok).toBe(false)
  expect(api.validateOidcCallback(`#code=one_time&state=${pending.state}&server=https://evil.example`, pending, () => 15_000).ok).toBe(false)
  expect(api.validateOidcCallback(`#code=one_time&state=${pending.state}`, pending, () => 10_000 + api.PENDING_OIDC_TTL_MS + 1).ok).toBe(false)

  const consumed = api.consumePendingOidcLogin(() => 15_000)
  expect(consumed).toEqual(pending)
  expect(consumed.codeVerifier).toBeTruthy()
  expect(api.consumePendingOidcLogin(() => 15_000)).toBeNull()
  expect(api.validateOidcCallback(`jaz://auth/callback#code=one_time&state=${pending.state}`, consumed, () => 15_000)).toEqual({
    ok: true,
    server: 'https://cloud.jaz.example',
    code: 'one_time',
    token: '',
    codeVerifier: consumed.codeVerifier,
  })

  const challenge = await api.pkceS256Challenge(consumed.codeVerifier)
  expect(
    api.buildOidcLoginUrl('https://cloud.jaz.example', '/v1/auth/login', {
      clientRedirect: 'jaz://auth/callback',
      state: pending.state,
      codeChallenge: challenge,
    }),
  ).toContain(`code_challenge=${encodeURIComponent(challenge)}`)

  replace('fetch', async (url, init) => {
    expect(String(url)).toBe('https://cloud.jaz.example/v1/auth/exchange')
    expect(init.method).toBe('POST')
    expect(init.redirect).toBe('error')
    expect(JSON.parse(String(init.body))).toEqual({ code: 'one_time', code_verifier: consumed.codeVerifier })
    return Response.json({
      token: 'session_jwt_1',
      user: {
        id: 'usr_1',
        org_id: 'org_acme',
        org_name: 'Acme',
        email: 'member@acme.example',
        display_name: 'Alex',
        role: 'member',
      },
      resource_ticket: { token: 'rt_exchanged', expires_at: '2099-01-01T00:00:00Z' },
    })
  })
  const exchanged = await api.exchangeOidcCode('https://cloud.jaz.example', 'one_time', undefined, consumed.codeVerifier)
  expect(exchanged).toEqual({
    token: 'session_jwt_1',
    user: {
      id: 'usr_1',
      org_id: 'org_acme',
      org_name: 'Acme',
      email: 'member@acme.example',
      display_name: 'Alex',
      role: 'member',
    },
    ticket: { token: 'rt_exchanged', expires_at: '2099-01-01T00:00:00Z' },
  })
  // Plain JWT string resource_ticket is also accepted for backward compatibility.
  const stringTicket = api.extractResourceTicket({ resource_ticket: 'rt_plain_jwt_string' })
  expect(stringTicket?.token).toBe('rt_plain_jwt_string')
  expect(stringTicket?.expires_at).toBeTruthy()
})

test('Spaces, Thread User State, Instructions body_md + 409 conflict, RBAC, and Connected Machines match real jaz-cloud contracts', async () => {
  const { api } = fixture()
  const calls = []
  replace('fetch', async (url, init) => {
    const parsed = new URL(String(url))
    const body = init?.body ? JSON.parse(String(init.body)) : undefined
    calls.push({ method: init?.method || 'GET', path: parsed.pathname, body })

    if (parsed.pathname === '/v1/auth/me') {
      return Response.json({
        id: 'usr_1',
        org_id: 'org_acme',
        org_name: 'Acme',
        email: 'member@acme.example',
        display_name: 'Alex',
        role: 'member',
        resource_ticket: { token: 'rt_me_1', expires_at: '2099-01-01T00:00:00Z' },
      })
    }
    if (parsed.pathname === '/v1/spaces' && (init?.method || 'GET') === 'GET') {
      return Response.json({ spaces: [{ id: 'spc_1', name: 'Engineering', member_count: 4 }] })
    }
    if (parsed.pathname === '/v1/spaces' && init?.method === 'POST') {
      return Response.json({ id: 'spc_2', name: body.name })
    }
    if (parsed.pathname === '/v1/spaces/spc_2/members' && init?.method === 'POST') {
      expect(body).toEqual({ email: 'teammate@acme.example', role: 'admin' })
      return Response.json({ ok: true, space_id: 'spc_2', user_id: 'usr_2', email: body.email, role: body.role })
    }
    if (parsed.pathname === '/v1/threads/thr_1/user-state' && init?.method === 'PUT') {
      expect(body).toEqual({ personal_sources_enabled: true })
      return Response.json({ ok: true, thread_id: 'thr_1', personal_sources_enabled: true })
    }
    if (parsed.pathname === '/v1/instructions/org/org_acme/agents' && (init?.method || 'GET') === 'GET') {
      return Response.json({
        org_id: 'org_acme',
        scope: 'org',
        scope_id: 'org_acme',
        kind: 'agents',
        body_md: '# Org Rules',
        version: 'v1_sha256',
      })
    }
    if (parsed.pathname === '/v1/instructions/org/org_acme/agents' && init?.method === 'PUT') {
      expect(body).toEqual({ body_md: '# Updated', expected_version: 'v1_sha256' })
      return Response.json(
        {
          error: 'version conflict',
          current: {
            org_id: 'org_acme',
            scope: 'org',
            scope_id: 'org_acme',
            kind: 'agents',
            body_md: '# Concurrent Org Rules',
            version: 'v2_sha256',
          },
        },
        { status: 409 },
      )
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
    user: {
      id: 'usr_1',
      org_id: 'org_acme',
      org_name: 'Acme',
      email: 'member@acme.example',
      display_name: 'Alex',
      role: 'member',
    },
    ticket: { token: 'rt_me_1', expires_at: '2099-01-01T00:00:00Z' },
  })

  expect(api.INSTRUCTION_SCOPE_KINDS.user).toEqual(['agents', 'soul'])
  expect(api.canEditCloudInstruction({ id: 'u', org_id: 'o', email: 'v@x', display_name: 'V', role: 'viewer' }, 'user')).toBe(false)
  expect(api.canEditCloudInstruction({ id: 'u', org_id: 'o', email: 'm@x', display_name: 'M', role: 'member' }, 'org')).toBe(false)
  expect(api.canEditCloudInstruction({ id: 'u', org_id: 'o', email: 'm@x', display_name: 'M', role: 'member' }, 'bot')).toBe(false)
  expect(api.canEditCloudInstruction({ id: 'u', org_id: 'o', email: 'm@x', display_name: 'M', role: 'member' }, 'user')).toBe(true)
  expect(api.canEditCloudInstruction({ id: 'u', org_id: 'o', email: 'a@x', display_name: 'A', role: 'admin' }, 'org')).toBe(true)

  expect(await api.listSpaces()).toEqual([{ id: 'spc_1', name: 'Engineering', member_count: 4 }])
  expect(await api.createSpace({ name: 'Platform' })).toEqual({ id: 'spc_2', name: 'Platform' })
  expect(await api.addSpaceMember('spc_2', { email: 'teammate@acme.example', role: 'admin' })).toMatchObject({
    space_id: 'spc_2',
    user_id: 'usr_2',
    email: 'teammate@acme.example',
    role: 'admin',
  })
  expect(await api.updateThreadUserState('thr_1', { personal_sources_enabled: true })).toMatchObject({
    thread_id: 'thr_1',
    personal_sources_enabled: true,
  })

  const doc = await api.getInstruction('org', 'org_acme', 'agents')
  expect(doc.body_md).toBe('# Org Rules')
  expect(doc.version).toBe('v1_sha256')

  let conflictError
  try {
    await api.saveInstruction('org', 'org_acme', 'agents', { body_md: '# Updated', expected_version: doc.version })
  } catch (error) {
    conflictError = error
  }
  expect(conflictError).toBeInstanceOf(api.ApiError)
  expect(conflictError.status).toBe(409)
  const currentDoc = api.extractInstructionConflictCurrent(conflictError, 'org', 'org_acme', 'agents')
  expect(currentDoc?.body_md).toBe('# Concurrent Org Rules')
  expect(currentDoc?.version).toBe('v2_sha256')

  const machines = await api.listMachines()
  expect(machines).toHaveLength(1)
  expect(machines[0].folder_grants).toEqual([{ id: 'repo', root_path: '/Users/dev/acme', mode: 'rw' }])
})

test('uploadCloudFile uses put_url, credentials: omit, ArrayBuffer body, SHA-256 commit, and resource_ticket file URL', async () => {
  const { api } = fixture()
  const steps = []
  const progress = []
  replace('fetch', async (url, init) => {
    const rawUrl = String(url)
    if (rawUrl.endsWith('/v1/runs/run_42/uploads/presign')) {
      const body = JSON.parse(String(init.body))
      steps.push({ step: 'presign', body, auth: new Headers(init.headers).get('Authorization') })
      return Response.json({
        put_url: 'https://storage.googleapis.com/jaz-bucket/signed-put?sig=1',
        headers: { 'x-goog-content-length-range': '0,1048576' },
        object_key: 'org_acme/u1/run_42/objects/deadbeef',
        input_id: 'inp_99',
        relpath: body.relpath,
        content_type: body.content_type,
      })
    }
    if (rawUrl.startsWith('https://storage.googleapis.com/')) {
      const headers = new Headers(init.headers)
      steps.push({
        step: 'put',
        method: init.method,
        credentials: init.credentials,
        isArrayBuffer: init.body instanceof ArrayBuffer,
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
        run_id: 'run_42',
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
  expect(Object.keys(steps[0].body).sort()).toEqual(['content_type', 'relpath', 'sha256'])
  expect(steps[0].body.sha256).toHaveLength(64)
  expect(steps[1]).toEqual({
    step: 'put',
    method: 'PUT',
    credentials: 'omit',
    isArrayBuffer: true,
    auth: null,
    contentType: file.type,
    googHeader: '0,1048576',
  })
  expect(steps[2].body.sha256).toBe(steps[0].body.sha256)
  expect(steps[2].body.input_id).toBe('inp_99')
  expect(attachment).toEqual({
    id: 'inp_99',
    name: 'notes.txt',
    uri: 'https://cloud.jaz.example/v1/runs/run_42/files/notes.txt?ticket=rt_signed_abc',
    mime_type: file.type,
    size: 11,
  })
  expect(progress.at(-1)).toBe(1)
})

test('fetchUsageSummary, evaluateSpendStatus, and isSpendCapError match real org_spent_usd/org_monthly_cap_usd schema', async () => {
  const { api } = fixture()
  replace('fetch', async (url) => {
    expect(new URL(String(url)).pathname).toBe('/v1/usage/summary')
    return Response.json({
      month_start: '2026-11-01T00:00:00Z',
      org_spent_usd: 85,
      org_monthly_cap_usd: 100,
      user_spent_usd: 12.5,
      user_monthly_cap_usd: 50,
    })
  })
  const summary = await api.fetchUsageSummary()
  expect(summary).toEqual({
    month_start: '2026-11-01T00:00:00Z',
    org_spent_usd: 85,
    org_monthly_cap_usd: 100,
    user_spent_usd: 12.5,
    user_monthly_cap_usd: 50,
  })
  expect(api.evaluateSpendStatus(summary)).toMatchObject({
    level: 'warning',
    scope: 'org',
    percent: 85,
  })
  expect(api.evaluateSpendStatus({
    month_start: '2026-11-01T00:00:00Z',
    org_spent_usd: 20,
    org_monthly_cap_usd: 100,
    user_spent_usd: 0,
    user_monthly_cap_usd: null,
  })).toBeNull()
  expect(api.evaluateSpendStatus({
    month_start: '2026-11-01T00:00:00Z',
    org_spent_usd: 50,
    org_monthly_cap_usd: 200,
    user_spent_usd: 25,
    user_monthly_cap_usd: 25,
  })).toMatchObject({
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

test('cloud model profiles read /v1/models { models } including string[] models (Vertex AI) and map into picker agent options', async () => {
  const { api } = fixture()
  replace('fetch', async (url) => {
    expect(new URL(String(url)).pathname).toBe('/v1/models')
    return Response.json({
      models: [
        { profile_id: 'openai-prod', provider: 'openai', default_model: 'gpt-5.4', default_effort: 'high' },
        { profile_id: 'anthropic-prod', provider: 'anthropic', default_model: 'claude-opus-4-6' },
        { profile_id: 'xai-prod', provider: 'xai', default_model: 'grok-4' },
        {
          profile_id: 'vertex-gemini',
          provider: 'vertex',
          display_name: 'Vertex AI',
          agent: 'codex',
          default_model: 'google/gemini-3-flash-preview',
          default_effort: 'high',
          models: ['google/gemini-3-flash-preview', 'google/gemini-3.1-pro-preview'],
        },
      ],
    })
  })
  const profiles = await api.listCloudModelProfiles()
  expect(profiles[0].models?.[0].reasoning?.status).toBe('ready')
  expect(profiles[3].models?.map((m) => [m.value, m.label])).toEqual([
    ['google/gemini-3-flash-preview', 'Gemini 3 Flash'],
    ['google/gemini-3.1-pro-preview', 'Gemini 3.1 Pro'],
  ])
  const cloudItems = profiles.map(api.cloudProfileToAgentOption)
  expect(cloudItems).toEqual([
    { id: 'openai-prod', label: 'Codex', logoAgent: 'codex', provider: 'openai' },
    { id: 'anthropic-prod', label: 'Claude', logoAgent: 'claude', provider: 'anthropic' },
    { id: 'xai-prod', label: 'Grok', logoAgent: 'grok', provider: 'xai' },
    { id: 'vertex-gemini', label: 'Vertex AI', logoAgent: 'codex', provider: 'vertex' },
  ])
})

test('Cloud mode routes session listing, creation, messages, and turns to /v1/threads endpoints instead of /v1/sessions', async () => {
  const { api } = fixture()
  const paths = []
  replace('fetch', async (url, init) => {
    const parsed = new URL(String(url))
    paths.push(`${init?.method || 'GET'} ${parsed.pathname}${parsed.search}`)
    if (parsed.pathname === '/v1/threads' && (init?.method || 'GET') === 'GET') {
      return Response.json({
        threads: [{
          id: 'thr_1',
          run_id: 'run_1',
          visibility: 'space',
          space_id: 'spc_1',
          title: 'Q4 Audit',
          status: 'idle',
          updated_at: '2026-11-22T12:00:00Z',
        }],
      })
    }
    if (parsed.pathname === '/v1/threads' && init?.method === 'POST') {
      const body = JSON.parse(String(init.body))
      return Response.json({
        id: 'thr_2',
        run_id: 'run_2',
        visibility: body.space_id ? 'space' : 'private',
        space_id: body.space_id ?? null,
        title: body.title ?? '',
        status: 'idle',
        updated_at: '2026-11-22T12:05:00Z',
      })
    }
    if (parsed.pathname === '/v1/threads/thr_2' && (init?.method || 'GET') === 'GET') {
      return Response.json({
        id: 'thr_2',
        run_id: 'run_2',
        visibility: 'private',
        title: 'Cloud Check',
        status: 'idle',
        updated_at: '2026-11-22T12:05:00Z',
      })
    }
    if (parsed.pathname === '/v1/threads/thr_2/events') {
      const sse = [
        'id: 1\nevent: user_message\ndata: {"seq":1,"thread_id":"thr_2","type":"user_message","payload":{"text":"Hello cloud"},"created_at":"2026-11-22T12:05:01Z"}\n\n',
        'id: 2\nevent: assistant_delta\ndata: {"seq":2,"thread_id":"thr_2","type":"assistant_delta","payload":{"delta":"Hi "},"created_at":"2026-11-22T12:05:02Z"}\n\n',
        'id: 3\nevent: assistant_delta\ndata: {"seq":3,"thread_id":"thr_2","type":"assistant_delta","payload":{"delta":"there"},"created_at":"2026-11-22T12:05:03Z"}\n\n',
        'id: 4\nevent: turn_completed\ndata: {"seq":4,"thread_id":"thr_2","type":"turn_completed","payload":{"status":"completed"},"created_at":"2026-11-22T12:05:04Z"}\n\n',
      ].join('')
      return new Response(sse, { headers: { 'content-type': 'text/event-stream' } })
    }
    if (parsed.pathname === '/v1/threads/thr_2/turns' && init?.method === 'POST') {
      return Response.json({
        turn: { id: 'trn_1', thread_id: 'thr_2', status: 'queued' },
        replayed: false,
      })
    }
    throw new Error(`Unexpected ${init?.method || 'GET'} ${parsed.pathname}`)
  })

  const created = await api.createSession({ title: 'Cloud Check', model_profile_id: 'openai-prod' })
  expect(created.id).toBe('thr_2')
  expect(created.run_id).toBe('run_2')

  const listed = await api.sidebarSessionsQuery.queryFn({ signal: new AbortController().signal })
  expect(listed.map((s) => s.id)).toEqual(['thr_1'])

  const page = await api.getSessionMessagesPage('thr_2')
  expect(page.messages.map((m) => [m.role, m.content])).toEqual([
    ['user', 'Hello cloud'],
    ['assistant', 'Hi there'],
  ])
  expect(page.latest_event_seq).toBe(4)

  const turn = await api.createCloudTurn('thr_2', { text: 'Follow up' })
  expect(turn.turn.id).toBe('trn_1')
  expect(paths.every((p) => !p.includes('/v1/sessions'))).toBe(true)
})

test('Vertex AI defaults include Gemini 4 Hydrogen, Neon, Carbon and Gemini 3 models', async () => {
  const { api } = fixture()
  replace('fetch', async () =>
    Response.json({
      models: [
        {
          profile_id: 'vertex-prod',
          provider: 'vertex',
          display_name: 'Vertex AI',
          default_model: 'google/gemini-4-hydrogen',
          default_effort: 'high',
        },
      ],
    }),
  )
  const profiles = await api.listCloudModelProfiles()
  expect(profiles).toHaveLength(1)
  expect(profiles[0].default_model).toBe('google/gemini-4-hydrogen')
  expect(profiles[0].default_effort).toBe('high')
  expect(profiles[0].models?.map((m) => [m.value, m.label, m.reasoning?.default_effort])).toEqual([
    ['google/gemini-4-hydrogen', 'Gemini 4 Hydrogen', 'high'],
    ['google/gemini-4-neon', 'Gemini 4 Neon', 'high'],
    ['google/gemini-4-carbon', 'Gemini 4 Carbon', 'high'],
    ['google/gemini-3-flash-preview', 'Gemini 3 Flash', 'high'],
    ['google/gemini-3.1-pro-preview', 'Gemini 3.1 Pro', 'high'],
  ])
})

test('Option A mixed mode: Local primary + attached Cloud routes per thread, merges sidebar, and survives Cloud outage', async () => {
  const { api } = fixture()
  // Reset primary to Local and attach Cloud alongside it.
  api.setApiBaseUrl('http://127.0.0.1:5299')
  api.setApiServerMode('local')
  api.setApiAuthToken('http://127.0.0.1:5299', 'local-token')
  api.clearCloudThreads()
  api.setAttachedCloudSession({
    baseUrl: 'https://cloud.jaz.example',
    token: 'cloud-attached-token',
    ticket: { token: 'rt_attached', expires_at: '2099-01-01T00:00:00Z' },
    user: { id: 'u1', org_id: 'org_1', org_name: 'CAS', email: 'dev@cas.example', display_name: 'Dev', role: 'admin' },
  })

  let cloudDown = false
  const calls = []
  replace('fetch', async (url, init) => {
    const parsed = new URL(String(url))
    const method = init?.method || 'GET'
    calls.push({ origin: parsed.origin, method, path: parsed.pathname, auth: new Headers(init?.headers).get('Authorization') })

    if (parsed.origin === 'http://127.0.0.1:5299') {
      if (parsed.pathname === '/v1/sessions' && method === 'POST') {
        return Response.json({ id: 'loc_1', slug: 'loc_1', title: 'Local work', status: 'idle', runtime: 'acp', created_at: '2026-11-22T10:00:00Z', updated_at: '2026-11-22T10:00:00Z', last_attention_at: '2026-11-22T10:00:00Z' })
      }
      if (parsed.pathname === '/v1/sessions' && method === 'GET') {
        return Response.json({
          sessions: [{ id: 'loc_1', slug: 'loc_1', title: 'Local work', status: 'idle', runtime: 'acp', created_at: '2026-11-22T10:00:00Z', updated_at: '2026-11-22T12:00:00Z', last_attention_at: '2026-11-22T12:00:00Z' }],
        })
      }
      if (parsed.pathname === '/v1/sessions/loc_1/queue' && method === 'POST') {
        return Response.json({ id: 'loc_1' })
      }
      if (parsed.pathname === '/v1/sessions/thr_deep_link') {
        return Response.json({ error: 'not found' }, { status: 404 })
      }
      throw new Error(`Unexpected local ${method} ${parsed.pathname}`)
    }

    if (parsed.origin === 'https://cloud.jaz.example') {
      if (cloudDown) return Response.json({ error: 'unavailable' }, { status: 503 })
      if (parsed.pathname === '/v1/threads' && method === 'POST') {
        return Response.json({ id: 'thr_cloud_1', run_id: 'run_cloud_1', visibility: 'private', title: 'Cloud Hydrogen', status: 'idle', updated_at: '2026-11-22T12:10:00Z' })
      }
      if (parsed.pathname === '/v1/threads' && method === 'GET') {
        return Response.json({
          threads: [{ id: 'thr_cloud_1', run_id: 'run_cloud_1', visibility: 'private', title: 'Cloud Hydrogen', status: 'idle', updated_at: '2026-11-22T12:10:00Z' }],
        })
      }
      if (parsed.pathname === '/v1/threads/thr_cloud_1/turns' && method === 'POST') {
        return Response.json({ turn: { id: 'trn_c1', thread_id: 'thr_cloud_1', status: 'queued' }, replayed: false })
      }
      if (parsed.pathname === '/v1/threads/thr_cloud_1/user-state' && method === 'PUT') {
        return Response.json({ ok: true, thread_id: 'thr_cloud_1', pinned: true })
      }
      if (parsed.pathname === '/v1/threads/thr_cloud_1' && method === 'GET') {
        return Response.json({ id: 'thr_cloud_1', run_id: 'run_cloud_1', visibility: 'private', title: 'Cloud Hydrogen', status: 'idle', updated_at: '2026-11-22T12:10:00Z' })
      }
      if (parsed.pathname === '/v1/threads/thr_deep_link' && method === 'GET') {
        return Response.json({ id: 'thr_deep_link', run_id: 'run_dl', visibility: 'private', title: 'Deep Linked', status: 'idle', updated_at: '2026-11-22T11:00:00Z' })
      }
      throw new Error(`Unexpected cloud ${method} ${parsed.pathname}`)
    }

    throw new Error(`Unexpected origin ${parsed.origin}`)
  })

  // Create Local thread -> Local backend; Create Cloud thread (with model_profile_id) -> Cloud backend.
  const localCreated = await api.createSession({ title: 'Local work', directory: '/repo' })
  expect(localCreated.id).toBe('loc_1')
  expect(api.isCloudThread('loc_1')).toBe(false)

  const cloudCreated = await api.createSession({
    title: 'Cloud Hydrogen',
    model_profile_id: 'vertex-prod',
    model: 'google/gemini-4-hydrogen',
  })
  expect(cloudCreated.id).toBe('thr_cloud_1')
  expect(cloudCreated.backend).toBe('cloud')
  expect(api.isCloudThread('thr_cloud_1')).toBe(true)
  expect(api.apiBaseUrl()).toBe('http://127.0.0.1:5299')

  // Prompts route per-thread: Local -> /v1/sessions/loc_1/queue, Cloud -> /v1/threads/thr_cloud_1/turns.
  await api.appendSessionPrompt('loc_1', 'hello local', {}, 'initial')
  await api.appendSessionPrompt('thr_cloud_1', 'hello cloud', {}, 'initial', undefined, {
    runId: 'run_cloud_1',
    modelProfileId: 'vertex-prod',
  })
  await api.setSessionPinned('thr_cloud_1', true)

  // Sidebar merges Local + Cloud newest first.
  const merged = await api.sidebarSessionsQuery.queryFn({ signal: new AbortController().signal })
  expect(merged.map((s) => [s.id, s.backend ?? 'local'])).toEqual([
    ['thr_cloud_1', 'cloud'],
    ['loc_1', 'local'],
  ])

  // Transient Cloud outage never breaks Local sidebar.
  cloudDown = true
  const degraded = await api.sidebarSessionsQuery.queryFn({ signal: new AbortController().signal })
  expect(degraded.map((s) => s.id)).toEqual(['loc_1'])

  // Cold deep link to an unregistered Cloud thread falls back from Local 404 to Cloud and registers it.
  cloudDown = false
  expect(api.isCloudThread('thr_deep_link')).toBe(false)
  const deepLinked = await api.getSession('thr_deep_link')
  expect(deepLinked.id).toBe('thr_deep_link')
  expect(deepLinked.backend).toBe('cloud')
  expect(api.isCloudThread('thr_deep_link')).toBe(true)

  // Verify strict auth separation between Local and Cloud calls.
  expect(calls.filter((c) => c.origin === 'http://127.0.0.1:5299').every((c) => c.auth === 'Bearer local-token')).toBe(true)
  expect(calls.filter((c) => c.origin === 'https://cloud.jaz.example').every((c) => c.auth === 'Bearer cloud-attached-token')).toBe(true)
})
