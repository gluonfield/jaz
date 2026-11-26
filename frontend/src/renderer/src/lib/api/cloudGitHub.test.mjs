import { afterEach, expect, test } from 'bun:test'
import { build } from 'bun'
import { createRequire } from 'node:module'

const bundle = await build({
  entrypoints: ['cloud-github-fixture'],
  target: 'browser',
  format: 'cjs',
  define: { 'import.meta.env': '{}' },
  plugins: [{
    name: 'cloud-github-fixture',
    setup(builder) {
      builder.onResolve({ filter: /^cloud-github-fixture$/ }, () => ({ path: 'entry', namespace: 'fixture' }))
      builder.onResolve({ filter: /^\//, namespace: 'fixture' }, (args) => ({ path: args.path, namespace: 'file' }))
      builder.onLoad({ filter: /.*/, namespace: 'fixture' }, () => ({
        contents: [
          `export * from ${JSON.stringify(new URL('./cloudGitHub.ts', import.meta.url).pathname)}`,
          `export * from ${JSON.stringify(new URL('./cloudSpaces.ts', import.meta.url).pathname)}`,
          `export * from ${JSON.stringify(new URL('./cloudThreads.ts', import.meta.url).pathname)}`,
          `export * from ${JSON.stringify(new URL('./sessions.ts', import.meta.url).pathname)}`,
          `export * from ${JSON.stringify(new URL('./client.ts', import.meta.url).pathname)}`,
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

function setupCloudApi() {
  const stored = new Map()
  replace('window', {
    location: { href: 'https://cloud.jaz.example/', origin: 'https://cloud.jaz.example', pathname: '/', search: '', hash: '' },
    addEventListener() {},
    removeEventListener() {},
    jaz: undefined,
  })
  replace('localStorage', {
    getItem: (key) => stored.get(key) ?? null,
    setItem: (key, value) => stored.set(key, value),
    removeItem: (key) => stored.delete(key),
  })
  window.localStorage = localStorage

  const mod = { exports: {} }
  evaluate(require, mod, mod.exports)
  const api = mod.exports
  api.setApiBaseUrl('https://cloud.jaz.example')
  api.setApiServerMode('cloud')
  api.setApiAuthToken('https://cloud.jaz.example', 'test-cloud-token')
  return api
}

test('isCloudAuthExpiredError matches only HTTP 401 ApiError responses', () => {
  const api = setupCloudApi()
  expect(api.isCloudAuthExpiredError(new api.ApiError(401, 'not signed in to Jaz Cloud'))).toBe(true)
  expect(api.isCloudAuthExpiredError(new api.ApiError(403, 'forbidden'))).toBe(false)
  expect(api.isCloudAuthExpiredError(new api.ApiError(500, 'server error'))).toBe(false)
  expect(api.isCloudAuthExpiredError(new Error('not signed in to Jaz Cloud'))).toBe(false)
})

test('buildCloudGitHubPlugin maps configured, linked, viewer, unconfigured, and auth-expired states for Plugins', () => {
  const api = setupCloudApi()

  const unconfigured = api.buildCloudGitHubPlugin({
    status: { configured: false, user_linked: false, installations: [] },
  })
  expect(unconfigured).toMatchObject({
    id: 'github',
    origin: 'builtin',
    icon: { kind: 'asset', value: 'github' },
    auth: { kind: 'oauth', status: 'unavailable', connected: false },
  })
  expect(unconfigured.auth.reason).toContain('not configured')

  const available = api.buildCloudGitHubPlugin({
    status: { configured: true, user_linked: false, installations: [] },
  })
  expect(available.auth).toMatchObject({ kind: 'oauth', status: 'available', connected: false, accounts: [] })

  const viewerUnlinked = api.buildCloudGitHubPlugin({
    status: { configured: true, user_linked: false, installations: [] },
    isViewer: true,
  })
  expect(viewerUnlinked.auth).toMatchObject({ status: 'unavailable', connected: false })
  expect(viewerUnlinked.auth.reason).toContain('View-only')

  const linked = api.buildCloudGitHubPlugin({
    status: { configured: true, user_linked: true, github_login: 'octocat', installations: [] },
  })
  expect(linked.auth).toMatchObject({
    status: 'connected',
    connected: true,
    accounts: [{ id: 'github-default', provider: 'github', alias: '@octocat', account_id: '@octocat' }],
  })

  const expired = api.buildCloudGitHubPlugin({ authExpired: true })
  expect(expired.auth).toMatchObject({ status: 'unavailable', connected: false })
  expect(expired.auth.reason).toContain('Sign in to Jaz Cloud')

  expect(api.withGitHubPluginSearch({ space: 'sp_1' })).toEqual({
    space: 'sp_1',
    settings: 'plugins',
    plugin: 'github',
  })
})

test('disconnectCloudGitHubAccount deletes only github connector accounts', async () => {
  const api = setupCloudApi()
  const deleted = []
  replace('fetch', async (url, init) => {
    const method = init?.method ?? 'GET'
    if (method === 'GET' && String(url) === 'https://cloud.jaz.example/v1/connectors') {
      return Response.json({
        connectors: [
          { id: 'conn_gh_1', server_id: 'github', alias: 'default' },
          { id: 'conn_slack_1', server_id: 'slack', alias: 'work' },
        ],
      })
    }
    if (method === 'DELETE') {
      deleted.push(String(url))
      return Response.json({ ok: true })
    }
    throw new Error(`unexpected ${method} ${url}`)
  })

  const res = await api.disconnectCloudGitHubAccount()
  expect(res).toEqual({ disconnected: 1 })
  expect(deleted).toEqual(['https://cloud.jaz.example/v1/connectors/conn_gh_1'])
})

test('listCloudGitHubBranches converts 428 connect_required into structured result and push parses connect error', async () => {
  const api = setupCloudApi()
  replace('fetch', async (url) => {
    if (String(url).includes('/branches')) {
      return new Response(
        JSON.stringify({
          error: 'link your GitHub account to browse branches',
          connect_required: true,
          connect_link: 'jaz://plugin/github/connect',
        }),
        { status: 428, headers: { 'Content-Type': 'application/json' } },
      )
    }
    return new Response(
      JSON.stringify({
        error: 'link your GitHub account before pushing',
        connect_required: true,
        connect_link: 'jaz://plugin/github/connect',
      }),
      { status: 428, headers: { 'Content-Type': 'application/json' } },
    )
  })

  const branches = await api.listCloudGitHubBranches('acme', 'app')
  expect(branches).toEqual({
    branches: [],
    connect_required: true,
    connect_link: 'jaz://plugin/github/connect',
  })

  let caught
  try {
    await api.pushCloudThreadRepo('th_1')
  } catch (err) {
    caught = err
  }
  expect(api.parseGitHubConnectError(caught)).toEqual({
    connectRequired: true,
    connectLink: 'jaz://plugin/github/connect',
    message: 'link your GitHub account before pushing',
  })
})

test('thread repo endpoints, compare-URL PR fallback, space defaults, and thread git_repo forward exact payloads', async () => {
  const api = setupCloudApi()
  const calls = []
  replace('fetch', async (url, init) => {
    const method = init?.method ?? 'GET'
    const body = init?.body ? JSON.parse(init.body) : undefined
    calls.push({ method, url: String(url), body })

    if (String(url).endsWith('/v1/threads/th_1/repo/pr')) {
      return Response.json({
        connect_required: true,
        connect_link: 'jaz://plugin/github/connect',
        compare_url: 'https://github.com/acme/app/compare/main...jaz%2Ffix?expand=1&title=Fix',
      })
    }
    if (String(url).endsWith('/v1/spaces')) {
      return Response.json({
        id: 'sp_1',
        name: 'Core',
        default_git_repo: body.default_git_repo,
        default_git_branch: body.default_git_branch,
      })
    }
    if (String(url).endsWith('/v1/threads')) {
      return Response.json({
        id: 'th_2',
        run_id: 'th_2',
        visibility: 'private',
        title: 'Bugfix',
        status: 'idle',
        git_repo: body.git_repo,
        git_base_branch: body.git_base_branch,
        updated_at: '2026-11-25T12:00:00Z',
      })
    }
    if (String(url).includes('/v1/threads/th_1/repo/diff')) {
      return Response.json({
        repo: 'acme/app',
        base_branch: 'main',
        branch: 'jaz/fix',
        diffs: [{ path: 'src/a.ts', status: 'modified', source: 'remote_compare', patch: '@@ -1 +1 @@\n-a\n+b' }],
      })
    }
    return Response.json({ ok: true })
  })

  const prFallback = await api.createCloudThreadRepoPR('th_1', { title: 'Fix' })
  expect(prFallback.connect_required).toBe(true)
  expect(prFallback.compare_url).toContain('github.com/acme/app/compare/main...jaz%2Ffix')

  const diff = await api.getCloudThreadRepoDiff('th_1', 'src/a.ts')
  expect(diff.diffs).toHaveLength(1)
  expect(calls.at(-1)?.url).toBe('https://cloud.jaz.example/v1/threads/th_1/repo/diff?path=src%2Fa.ts')

  const space = await api.createSpace({
    name: 'Core',
    default_git_repo: 'acme/app',
    default_git_branch: 'main',
  })
  expect(space.default_git_repo).toBe('acme/app')
  expect(space.default_git_branch).toBe('main')

  const session = await api.createSession({
    title: 'Bugfix',
    git_repo: 'acme/app',
    git_base_branch: 'develop',
  })
  expect(session.git_repo).toBe('acme/app')
  expect(session.git_base_branch).toBe('develop')
})

test('GitHub URL and segment guards reject unsafe schemes and invalid org/repo names', async () => {
  const api = setupCloudApi()
  const opened = []
  window.open = (url) => opened.push(url)

  expect(api.isValidGitHubSegment('acme-corp')).toBe(true)
  expect(api.isValidGitHubSegment('..')).toBe(false)
  expect(api.isValidGitHubSegment('bad/org')).toBe(false)
  expect(api.githubRepoWebUrl('acme/app.js')).toBe('https://github.com/acme/app.js')
  expect(api.githubRepoWebUrl('acme/../evil')).toBe('')

  expect(() => api.openSafeGitHubUrl('javascript:alert(1)')).toThrow()
  expect(() => api.openSafeGitHubUrl('http://github.com/acme/app')).toThrow()
  api.openSafeGitHubUrl('https://github.com/acme/app/pull/42')
  expect(opened).toEqual(['https://github.com/acme/app/pull/42'])
})
