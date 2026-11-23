import { afterEach, expect, test } from 'bun:test'
import { build } from 'bun'
import { createRequire } from 'node:module'

const bundle = await build({
  entrypoints: ['machine-bridge-fixture'],
  target: 'browser',
  format: 'cjs',
  define: { 'import.meta.env': '{}' },
  plugins: [{
    name: 'machine-bridge-fixture',
    setup(builder) {
      builder.onResolve({ filter: /^machine-bridge-fixture$/ }, () => ({ path: 'entry', namespace: 'fixture' }))
      builder.onResolve({ filter: /^\//, namespace: 'fixture' }, (args) => ({ path: args.path, namespace: 'file' }))
      builder.onLoad({ filter: /.*/, namespace: 'fixture' }, () => ({
        contents: [
          `export * from ${JSON.stringify(new URL('./machineBridge.ts', import.meta.url).pathname)}`,
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

function setupApi() {
  replace('window', {
    location: {
      href: 'http://127.0.0.1:5299/',
      origin: 'http://127.0.0.1:5299',
      pathname: '/',
      search: '',
      hash: '',
    },
    addEventListener() {},
    jaz: {
      apiBaseUrl: 'http://127.0.0.1:5299',
      cloudUrl: '',
      localEnabled: true,
      windowKind: 'main',
    },
  })
  const module = { exports: {} }
  evaluate(require, module, module.exports)
  const api = module.exports
  api.setApiBaseUrl('http://127.0.0.1:5299')
  api.setApiAuthToken('http://127.0.0.1:5299', 'local-secret')
  return api
}

test('getMachineBridge fetches status and normalizes nullable fields', async () => {
  const api = setupApi()
  let seenAuth = ''
  replace('fetch', async (url, init) => {
    expect(String(url)).toBe('http://127.0.0.1:5299/v1/cloud/machine-bridge')
    seenAuth = new Headers(init?.headers).get('Authorization') ?? ''
    return Response.json({
      device_id: 'jaz-abc123',
      connected: true,
      cloud_url: 'https://cloud.jaz.example',
      config: {
        enabled: true,
        display_name: 'Studio Mac',
        grants: null,
        allow_exec: false,
      },
    })
  })

  const status = await api.getMachineBridge()
  expect(seenAuth).toBe('Bearer local-secret')
  expect(status).toEqual({
    device_id: 'jaz-abc123',
    connected: true,
    cloud_url: 'https://cloud.jaz.example',
    last_connected_at: null,
    last_error: '',
    config: {
      enabled: true,
      display_name: 'Studio Mac',
      grants: [],
      allow_exec: false,
    },
  })
})

test('updateMachineBridge sends PUT JSON and propagates validation errors', async () => {
  const api = setupApi()
  const requests = []
  replace('fetch', async (url, init) => {
    const body = JSON.parse(String(init?.body ?? '{}'))
    requests.push({ url: String(url), method: init?.method, body })
    if (body.grants?.[0]?.root_path === '/missing') {
      return Response.json({ error: 'invalid_request: folder "/missing": no such file or directory' }, { status: 400 })
    }
    return Response.json({
      device_id: 'jaz-abc123',
      connected: true,
      cloud_url: 'https://cloud.jaz.example',
      last_connected_at: '2026-11-23T12:00:00Z',
      config: body,
    })
  })

  const saved = await api.updateMachineBridge({
    enabled: true,
    display_name: 'Office Mac',
    allow_exec: true,
    grants: [{ id: 'work', root_path: '/Users/dev/work', mode: 'rw' }],
  })
  expect(requests[0]).toEqual({
    url: 'http://127.0.0.1:5299/v1/cloud/machine-bridge',
    method: 'PUT',
    body: {
      enabled: true,
      display_name: 'Office Mac',
      allow_exec: true,
      grants: [{ id: 'work', root_path: '/Users/dev/work', mode: 'rw' }],
    },
  })
  expect(saved.config.allow_exec).toBe(true)
  expect(saved.last_connected_at).toBe('2026-11-23T12:00:00Z')

  await expect(api.updateMachineBridge({
    enabled: true,
    display_name: 'Office Mac',
    allow_exec: false,
    grants: [{ id: 'bad', root_path: '/missing', mode: 'ro' }],
  })).rejects.toThrow('folder "/missing"')
})
