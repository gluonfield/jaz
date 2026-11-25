import { afterEach, expect, test } from 'bun:test'
import { build } from 'bun'
import { createRequire } from 'node:module'

const bundle = await build({
  entrypoints: ['cloud-run-files-fixture'],
  target: 'browser',
  format: 'cjs',
  define: { 'import.meta.env': '{}' },
  plugins: [{
    name: 'cloud-run-files-fixture',
    setup(builder) {
      builder.onResolve({ filter: /^cloud-run-files-fixture$/ }, () => ({ path: 'entry', namespace: 'fixture' }))
      builder.onResolve({ filter: /^\//, namespace: 'fixture' }, (args) => ({ path: args.path, namespace: 'file' }))
      builder.onLoad({ filter: /.*/, namespace: 'fixture' }, () => ({
        contents: [
          `export * from ${JSON.stringify(new URL('./cloudUploads.ts', import.meta.url).pathname)}`,
          `export * from ${JSON.stringify(new URL('./cloudThreads.ts', import.meta.url).pathname)}`,
          `export * from ${JSON.stringify(new URL('./sessions.ts', import.meta.url).pathname)}`,
          `export * from ${JSON.stringify(new URL('./client.ts', import.meta.url).pathname)}`,
          `export * from ${JSON.stringify(new URL('../markdownImages.ts', import.meta.url).pathname)}`,
          `export { isCloudViewerRole } from ${JSON.stringify(new URL('../cloudSession.ts', import.meta.url).pathname)}`,
          `export * from ${JSON.stringify(new URL('../../components/loops/loopDraft.ts', import.meta.url).pathname)}`,
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

function fixture() {
  const stored = new Map()
  const loc = new URL('https://cloud.jaz.example/')
  replace('window', {
    location: {
      href: loc.href,
      origin: loc.origin,
      pathname: loc.pathname,
      search: loc.search,
      hash: loc.hash,
    },
    history: { state: null, replaceState() {} },
    addEventListener() {},
  })
  replace('navigator', { platform: 'MacIntel' })
  replace('localStorage', {
    getItem(key) {
      return stored.has(key) ? stored.get(key) : null
    },
    setItem(key, value) {
      stored.set(key, String(value))
    },
    removeItem(key) {
      stored.delete(key)
    },
  })

  const mod = { exports: {} }
  evaluate(require, mod, mod.exports)
  const api = mod.exports
  api.setApiBaseUrl('https://cloud.jaz.example')
  api.setApiServerMode('cloud')
  api.setApiAuthToken('https://cloud.jaz.example', 'cloud-jwt', {
    token: 'rt_signed_abc',
    expires_at: new Date(Date.now() + 600_000).toISOString(),
  })
  return api
}

test('Cloud run file helpers normalize workspace paths, use ticket auth with credentials omit, and route session file APIs', async () => {
  const api = fixture()
  const requests = []
  replace('fetch', async (url, init = {}) => {
    const parsed = new URL(String(url))
    requests.push({
      pathname: parsed.pathname,
      ticket: parsed.searchParams.get('ticket'),
      credentials: init.credentials,
      auth: init.headers ? new Headers(init.headers).get('Authorization') : null,
    })
    if (parsed.pathname === '/v1/runs/run_99/files/docs/report.md') {
      return new Response('# Cloud Report\n\nReady.', {
        status: 200,
        headers: { 'Content-Type': 'text/markdown', ETag: '"etag-md-1"' },
      })
    }
    if (parsed.pathname === '/v1/runs/run_99/files/assets/chart.bin') {
      return new Response(new Uint8Array([0x89, 0x50, 0x4e, 0x47, 0x00, 0x01]), {
        status: 200,
        headers: { 'Content-Type': 'application/octet-stream' },
      })
    }
    return new Response('not found', { status: 404 })
  })

  expect(api.normalizeCloudRunRelpath('/workspace/docs/report.md')).toBe('docs/report.md')
  expect(api.normalizeCloudRunRelpath('file:///workspace/docs/report.md#heading')).toBe('docs/report.md')
  expect(api.normalizeCloudRunRelpath('./docs/../docs/report.md?raw=1')).toBe('docs/report.md')
  expect(() => api.normalizeCloudRunRelpath('../etc/passwd')).toThrow('invalid file path')

  const doc = await api.readSessionFile('thr_99', '/workspace/docs/report.md', undefined, 'run_99')
  expect(doc).toEqual({
    path: 'docs/report.md',
    relative_path: 'docs/report.md',
    size: 22,
    binary: false,
    truncated: false,
    content: '# Cloud Report\n\nReady.',
    version: '"etag-md-1"',
    read_only: true,
  })

  const bin = await api.readSessionFile('thr_99', 'assets/chart.bin', undefined, 'run_99')
  expect(bin.binary).toBe(true)
  expect(bin.content).toBeUndefined()
  expect(bin.read_only).toBe(true)

  const bytes = await api.sessionFileBytes('thr_99', 'assets/chart.bin', undefined, 'run_99')
  expect(new Uint8Array(bytes)).toEqual(new Uint8Array([0x89, 0x50, 0x4e, 0x47, 0x00, 0x01]))

  expect(requests).toEqual([
    { pathname: '/v1/runs/run_99/files/docs/report.md', ticket: 'rt_signed_abc', credentials: 'omit', auth: null },
    { pathname: '/v1/runs/run_99/files/assets/chart.bin', ticket: 'rt_signed_abc', credentials: 'omit', auth: null },
    { pathname: '/v1/runs/run_99/files/assets/chart.bin', ticket: 'rt_signed_abc', credentials: 'omit', auth: null },
  ])

  expect(api.sessionFileRawUrl('thr_99', '/workspace/assets/chart.png', 'run_99')).toBe(
    'https://cloud.jaz.example/v1/runs/run_99/files/assets/chart.png?ticket=rt_signed_abc',
  )
  expect(api.sessionFileDownloadUrl('thr_99', '/workspace/docs/report.md', 'run_99')).toBe(
    'https://cloud.jaz.example/v1/runs/run_99/files/docs/report.md?download=1&ticket=rt_signed_abc',
  )
  expect(api.markdownImageSource('chart.png', 'img1', 'thr_99', 'docs/report.md', 'run_99')).toBe(
    'https://cloud.jaz.example/v1/runs/run_99/files/docs/chart.png?ticket=rt_signed_abc&image=img1',
  )

  // Attached Desktop mode (local server mode) still routes Cloud sessions to /v1/runs/{runId}/files/...
  api.setApiServerMode('local')
  expect(api.sessionCloudRunId({ id: 'thr_1', backend: 'cloud', run_id: 'run_1' })).toBe('run_1')
  expect(api.sessionCloudRunId({ id: 'loc_1', backend: 'local' })).toBeUndefined()
  expect(api.sessionFileRawUrl('thr_1', '/workspace/out.png', 'run_1')).toBe(
    'https://cloud.jaz.example/v1/runs/run_1/files/out.png?ticket=rt_signed_abc',
  )
  expect(api.sessionFileRawUrl('loc_1', '/tmp/local.png')).toContain('/v1/sessions/loc_1/file?')
})

test('Cloud run file fetch falls back to authenticated apiFetch when resource ticket is expired or rejected', async () => {
  const api = fixture()
  const seen = []
  replace('fetch', async (url, init = {}) => {
    const parsed = new URL(String(url))
    const auth = init.headers ? new Headers(init.headers).get('Authorization') : null
    seen.push({ ticket: parsed.searchParams.get('ticket'), auth })
    if (parsed.searchParams.get('ticket') === 'rt_revoked') {
      return new Response('{"error":"ticket expired"}', { status: 401 })
    }
    return new Response('ok', { status: 200 })
  })

  // Expired ticket skips query auth and goes straight to Bearer apiFetch.
  api.setResourceTicket('https://cloud.jaz.example', {
    token: 'rt_expired',
    expires_at: new Date(Date.now() - 60_000).toISOString(),
  })
  const expiredDoc = await api.readCloudRunFile('run_1', 'notes.md')
  expect(expiredDoc.content).toBe('ok')

  // Active ticket that gets a 401 falls back cleanly to Bearer apiFetch.
  api.setResourceTicket('https://cloud.jaz.example', {
    token: 'rt_revoked',
    expires_at: new Date(Date.now() + 60_000).toISOString(),
  })
  const revokedDoc = await api.readCloudRunFile('run_1', 'notes.md')
  expect(revokedDoc.content).toBe('ok')

  expect(seen).toEqual([
    { ticket: null, auth: 'Bearer cloud-jwt' },
    { ticket: 'rt_revoked', auth: null },
    { ticket: null, auth: 'Bearer cloud-jwt' },
  ])
})

test('resolveCloudModelSelection, loopDraftToInput, and isCloudViewerRole handle Cloud-only profiles and RBAC', () => {
  const api = fixture()
  const profiles = [
    {
      profile_id: 'openai-org',
      provider: 'openai',
      display_name: 'Codex Org',
      agent: 'codex',
      default_model: 'gpt-6.1-sol',
      default_effort: 'high',
      models: [
        { value: 'gpt-6.1-sol', label: 'gpt-6.1-sol', reasoning: { efforts: ['low', 'medium', 'high'], default_effort: 'high', status: 'ready' } },
      ],
    },
    {
      profile_id: 'vertex-org',
      provider: 'vertex',
      display_name: 'Vertex AI',
      agent: 'gemini',
      default_model: 'google/gemini-4-hydrogen',
      default_effort: 'medium',
      models: [
        { value: 'google/gemini-4-hydrogen', label: 'gemini-4-hydrogen', reasoning: { efforts: ['low', 'medium', 'high'], default_effort: 'medium', status: 'ready' } },
      ],
    },
  ]

  const resolved = api.resolveCloudModelSelection(profiles, { profileId: 'vertex-org', effort: 'unsupported' })
  expect(resolved.profileId).toBe('vertex-org')
  expect(resolved.model).toBe('google/gemini-4-hydrogen')
  expect(resolved.effort).toBe('medium')
  expect(resolved.agents).toEqual(['openai-org', 'vertex-org'])

  const byAgent = api.resolveCloudModelSelection(profiles, { agent: 'gemini' })
  expect(byAgent.profileId).toBe('vertex-org')

  const draft = {
    ...api.emptyLoopDraft(['b1']),
    prompt: 'Check status',
    runtime: 'vertex-org',
    provider: 'vertex-org',
    model: 'google/gemini-4-hydrogen',
    reasoningEffort: 'high',
  }
  const input = api.loopDraftToInput(draft, undefined, profiles)
  expect(input).toMatchObject({
    prompt: 'Check status',
    runtime: 'acp',
    acp_agent: 'gemini',
    model_provider: 'vertex-org',
    model: 'google/gemini-4-hydrogen',
    reasoning_effort: 'high',
    board_ids: ['b1'],
  })

  expect(api.isCloudViewerRole('viewer')).toBe(true)
  expect(api.isCloudViewerRole(' VIEWER ')).toBe(true)
  expect(api.isCloudViewerRole('member')).toBe(false)
  expect(api.isCloudViewerRole('admin')).toBe(false)
  expect(api.isCloudViewerRole(undefined)).toBe(false)
})

test('readCloudRunFile sends Range bytes=0-1048576, parses 206 Content-Range totals, and handles 416 empty files', async () => {
  const api = fixture()
  const ranges = []
  replace('fetch', async (url, init = {}) => {
    const parsed = new URL(String(url))
    const headers = new Headers(init.headers)
    ranges.push({ pathname: parsed.pathname, range: headers.get('Range') })
    if (parsed.pathname === '/v1/runs/run_big/files/models/large.glb') {
      const sample = new Uint8Array(1024 * 1024 + 1)
      sample[0] = 0x67
      sample[4] = 0x00
      return new Response(sample, {
        status: 206,
        headers: {
          'Content-Type': 'model/gltf-binary',
          'Content-Range': 'bytes 0-1048576/196083712',
          ETag: '"glb-etag-1"',
        },
      })
    }
    if (parsed.pathname === '/v1/runs/run_big/files/empty.txt') {
      return new Response(null, {
        status: 416,
        headers: { 'Content-Range': 'bytes */0', ETag: '"empty-etag"' },
      })
    }
    if (parsed.pathname === '/v1/runs/run_big/files/utf8.md') {
      // Split a 3-byte UTF-8 em-dash (0xE2 0x80 0x94) right across the 1 MiB truncation boundary.
      const sample = new Uint8Array(1024 * 1024 + 1)
      sample.fill(0x61)
      sample[1024 * 1024 - 1] = 0xe2
      sample[1024 * 1024] = 0x80
      return new Response(sample, {
        status: 206,
        headers: {
          'Content-Type': 'text/markdown',
          'Content-Range': 'bytes 0-1048576/2097152',
        },
      })
    }
    return new Response('not found', { status: 404 })
  })

  const big = await api.readCloudRunFile('run_big', 'models/large.glb')
  expect(big).toEqual({
    path: 'models/large.glb',
    relative_path: 'models/large.glb',
    size: 196083712,
    binary: true,
    truncated: true,
    version: '"glb-etag-1"',
    read_only: true,
  })

  const empty = await api.readCloudRunFile('run_big', 'empty.txt')
  expect(empty).toEqual({
    path: 'empty.txt',
    relative_path: 'empty.txt',
    size: 0,
    binary: false,
    truncated: false,
    content: '',
    version: '"empty-etag"',
    read_only: true,
  })

  const utf8 = await api.readCloudRunFile('run_big', 'utf8.md')
  expect(utf8.binary).toBe(false)
  expect(utf8.truncated).toBe(true)
  expect(utf8.size).toBe(2097152)
  expect(utf8.content?.length).toBe(1024 * 1024 - 1)

  expect(ranges).toEqual([
    { pathname: '/v1/runs/run_big/files/models/large.glb', range: 'bytes=0-1048576' },
    { pathname: '/v1/runs/run_big/files/empty.txt', range: 'bytes=0-1048576' },
    { pathname: '/v1/runs/run_big/files/utf8.md', range: 'bytes=0-1048576' },
  ])
})

test('grantSessionAsset routes Local and Cloud asset grants and computes active plugin asset origins', async () => {
  const api = fixture()
  const calls = []
  replace('fetch', async (url, init = {}) => {
    const parsed = new URL(String(url))
    calls.push({ method: init.method, url: parsed.toString(), body: init.body })
    if (parsed.pathname === '/v1/runs/run_77/assets/grant') {
      return new Response(JSON.stringify({
        url: 'https://storage.googleapis.com/jaz-bucket/obj?X-Goog-Signature=abc',
        size: 196083712,
        content_type: 'model/gltf-binary',
        etag: '"gcs-1"',
        expires_at: '2026-11-25T16:00:00Z',
      }), { status: 200, headers: { 'Content-Type': 'application/json' } })
    }
    if (parsed.pathname === '/v1/sessions/loc_77/assets/grant') {
      return new Response(JSON.stringify({
        url: 'http://127.0.0.1:5299/v1/sessions/loc_77/assets/outputs/part.glb?grant=ag_test.sig',
        size: 4096,
        content_type: 'model/gltf-binary',
        etag: '"loc-1"',
        expires_at: '2026-11-25T16:00:00Z',
      }), { status: 200, headers: { 'Content-Type': 'application/json' } })
    }
    return new Response('not found', { status: 404 })
  })

  const cloudGrant = await api.grantSessionAsset('thr_77', 'outputs/assembly.glb', undefined, 'run_77')
  expect(cloudGrant).toEqual({
    url: 'https://storage.googleapis.com/jaz-bucket/obj?X-Goog-Signature=abc',
    size: 196083712,
    contentType: 'model/gltf-binary',
    etag: '"gcs-1"',
    expiresAt: '2026-11-25T16:00:00Z',
  })
  expect(api.activePluginAssetOrigin('run_77')).toBe('https://storage.googleapis.com')

  api.setApiServerMode('local')
  api.setApiBaseUrl('http://127.0.0.1:5299')
  const localGrant = await api.grantSessionAsset('loc_77', 'outputs/part.glb')
  expect(localGrant).toEqual({
    url: 'http://127.0.0.1:5299/v1/sessions/loc_77/assets/outputs/part.glb?grant=ag_test.sig',
    size: 4096,
    contentType: 'model/gltf-binary',
    etag: '"loc-1"',
    expiresAt: '2026-11-25T16:00:00Z',
  })
  expect(api.activePluginAssetOrigin()).toBe('http://127.0.0.1:5299')
  expect(calls).toHaveLength(2)
})
