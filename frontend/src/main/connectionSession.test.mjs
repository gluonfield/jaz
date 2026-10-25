import { afterEach, expect, test } from 'bun:test'
import { build } from 'bun'
import { EventEmitter } from 'node:events'
import { createRequire } from 'node:module'
import { pathToFileURL } from 'node:url'

const bundle = await build({ entrypoints: [new URL('./connectionSession.ts', import.meta.url).pathname], target: 'node', format: 'cjs', external: ['electron'] })
if (!bundle.success) {
  throw new AggregateError(bundle.logs)
}
const evaluate = new Function('require', 'module', 'exports', await bundle.outputs[0].text())
const require = createRequire(import.meta.url)
const originalFetch = globalThis.fetch
const originalDevURL = process.env.ELECTRON_RENDERER_URL
afterEach(() => {
  globalThis.fetch = originalFetch
  if (originalDevURL === undefined) {
    delete process.env.ELECTRON_RENDERER_URL
  }
  else {
    process.env.ELECTRON_RENDERER_URL = originalDevURL
  }
})

function fixture({ start, registration, principal = 'device', deviceId = 'identity' } = {}) {
  delete process.env.ELECTRON_RENDERER_URL
  const rendererPath = '/fixture/app/out/renderer/index.html'
  const url = pathToFileURL(rendererPath).toString()
  const listeners = new Map()
  const invocations = new Map()
  const electron = { ipcMain: { on: (name, handler) => listeners.set(name, handler), handle: (name, handler) => invocations.set(name, handler) } }
  const module = { exports: {} }
  evaluate((name) => name === 'electron' ? electron : require(name), module, module.exports)
  let starts = 0
  let registrations = 0
  let stopped = 0
  const service = module.exports.createConnectionSession({
    rendererPath,
    startLocal: async () => {
      starts += 1
      return start ? start() : { ok: true, url: 'http://127.0.0.1:5299', key: 'root-grant' }
    },
    profile: () => ({ device_id: 'identity', public_key: 'public', name: 'Fixture Mac', platform: 'macOS', device_family: 'Mac', model_identifier: '', app_version: 'fixture' }),
    stopConsumers: () => {
      stopped += 1
    },
  })
  globalThis.fetch = async (target, init) => {
    expect(target.startsWith('http://127.0.0.1:5299/')).toBe(true)
    expect(init.redirect).toBe('error')
    if (target.endsWith('/health')) {
      return Response.json({ ok: true, auth_required: true })
    }
    if (target.endsWith('/register')) {
      registrations += 1
      expect(init.headers.Authorization).toBe('Bearer root-grant')
      return registration ? registration() : Response.json({ token: 'jaz_device-bearer' })
    }
    expect(init.headers.Authorization).toBe('Bearer jaz_device-bearer')
    return Response.json({ ok: true, auth_kind: principal, device_id: deviceId })
  }
  let nextId = 0
  function window(owner = false, type = 'window') {
    const contents = new EventEmitter()
    contents.id = ++nextId
    contents.getType = () => type
    contents.isDestroyed = () => false
    const value = new EventEmitter()
    value.webContents = contents
    value.isDestroyed = () => false
    value.messages = []
    value.navigate = (target = url) => {
      contents.emit('did-start-navigation', { isMainFrame: true, isSameDocument: false })
      contents.mainFrame = { url: target, send: (_name, message) => value.messages.push(message) }
      contents.emit('did-frame-navigate', null, target, 200, '', true)
    }
    service.attach(value, owner)
    value.navigate()
    value.event = () => ({ sender: contents, senderFrame: contents.mainFrame })
    value.call = (request, event = value.event()) => {
      listeners.get('jaz:connection-session')(event, request)
      if (event.returnValue.error) {
        throw new Error(event.returnValue.error)
      }
      return event.returnValue.value
    }
    value.read = () => value.call({ operation: 'read' })
    value.local = (document, revision) => invocations.get('jaz:local-session')(value.event(), document, revision)
    return value
  }
  return { service, window, url, counts: () => ({ starts, registrations, stopped }) }
}

test('handoff accepts actual app windows and exact documents, with owner-only publication', () => {
  const f = fixture()
  const owner = f.window(true)
  const board = f.window()
  const document = owner.read().document
  const secondary = board.read()
  expect(secondary.owner).toBe(false)
  expect(() => board.call({ operation: 'begin', document: secondary.document, url: 'https://remote.example' })).toThrow('owner')
  const pending = owner.call({ operation: 'begin', document, url: 'https://remote.example' })
  owner.call({ operation: 'commit', document, revision: pending.revision,
    session: { url: 'https://remote.example', token: 'jaz_authorized', deviceId: 'identity', remembered: false } })
  expect(board.messages.at(-1).snapshot.session.token).toBe('jaz_authorized')
  expect(Object.keys(board.read().snapshot.session).sort()).toEqual(['deviceId', 'remembered', 'token', 'url'])
  expect(() => owner.call({ operation: 'commit', document, revision: pending.revision,
    session: { url: 'https://other.example', token: 'jaz_other', deviceId: 'identity' } })).toThrow('replaced')
  board.navigate('file:///wrong/renderer/index.html')
  expect(() => board.read()).toThrow('document')
  const count = board.messages.length
  owner.call({ operation: 'clear', document, error: null })
  expect(board.messages).toHaveLength(count)
  expect(() => owner.call({ operation: 'read' }, { sender: owner.webContents, senderFrame: { url: f.url } })).toThrow('trusted')
  const guest = f.window(false, 'webview')
  expect(() => guest.read()).toThrow('trusted')
  const outsider = { ...owner.event(), sender: { id: 999, getType: () => 'window', mainFrame: owner.webContents.mainFrame } }
  expect(() => owner.call({ operation: 'read' }, outsider)).toThrow('document')
})

test('owner reload during local registration resumes one flight and rejects the old document', async () => {
  let release
  const started = new Promise((resolve) => {
    release = resolve
  })
  const f = fixture({ start: () => started })
  const owner = f.window(true)
  const first = owner.read()
  const pending = owner.call({ operation: 'begin', document: first.document, url: 'http://127.0.0.1:5299' })
  const old = owner.local(first.document, pending.revision).catch((error) => error)
  owner.navigate()
  const next = owner.read()
  expect(next.snapshot.localPending).toBe(true)
  const resumed = owner.call({ operation: 'begin', document: next.document, url: 'http://127.0.0.1:5299' })
  const result = owner.local(next.document, resumed.revision)
  release({ ok: true, url: 'http://127.0.0.1:5299', key: 'root-grant' })
  expect(await old).toBeInstanceOf(Error)
  const session = await result
  expect(session.token).toBe('jaz_device-bearer')
  expect(f.counts().starts).toBe(1)
  expect(f.counts().registrations).toBe(1)
  owner.call({ operation: 'commit', document: next.document, revision: resumed.revision, session })
  owner.navigate()
  expect(owner.read().snapshot.phase).toBe('connected')
  expect(() => owner.call({ operation: 'clear', document: first.document, error: null })).toThrow('document')
  owner.emit('closed')
  const recreated = f.window(true)
  expect(recreated.read().snapshot.session.token).toBe('jaz_device-bearer')
})

test('disconnect before startup completes skips registration and stays blocked after reload', async () => {
  let release
  const started = new Promise((resolve) => {
    release = resolve
  })
  const f = fixture({ start: () => started })
  const owner = f.window(true)
  const document = owner.read().document
  const pending = owner.call({ operation: 'begin', document, url: 'http://127.0.0.1:5299' })
  const result = owner.local(document, pending.revision).catch((error) => error)
  owner.call({ operation: 'clear', document, error: null })
  release({ ok: true, url: 'http://127.0.0.1:5299', key: 'root-grant' })
  expect(await result).toBeInstanceOf(Error)
  expect(f.counts().registrations).toBe(0)
  owner.navigate()
  expect(owner.read().snapshot.phase).toBe('blocked')
  expect(owner.read().snapshot.session).toBeNull()
  expect(f.counts().starts).toBe(1)
})

test.each([{ principal: 'root' }, { deviceId: 'another-device' }])('local activation rejects the wrong principal or identity: %j', async (options) => {
  const f = fixture(options)
  const owner = f.window(true)
  const document = owner.read().document
  const pending = owner.call({ operation: 'begin', document, url: 'http://127.0.0.1:5299' })
  await expect(owner.local(document, pending.revision)).rejects.toThrow('expected device')
  expect(owner.read().snapshot.phase).toBe('blocked')
  expect(owner.read().snapshot.session).toBeNull()
})

test('local root grant never follows an alternate loopback origin', async () => {
  const f = fixture({ start: async () => ({ ok: true, url: 'http://localhost:5299', key: 'root-grant' }) })
  const owner = f.window(true)
  const document = owner.read().document
  const pending = owner.call({ operation: 'begin', document, url: 'http://127.0.0.1:5299' })
  await expect(owner.local(document, pending.revision)).rejects.toThrow('wrong backend')
  expect(f.counts().registrations).toBe(0)
})


test('local to remote to local waits for outstanding issuance and discards its late result', async () => {
  let release
  let issued
  const response = new Promise((resolve) => {
    release = resolve
  })
  const started = new Promise((resolve) => {
    issued = resolve
  })
  let calls = 0
  const f = fixture({ registration: () => {
    calls += 1
    if (calls === 1) {
      issued()
      return response
    }
    return Response.json({ token: 'jaz_device-bearer' })
  } })
  const owner = f.window(true)
  const document = owner.read().document
  const first = owner.call({ operation: 'begin', document, url: 'http://127.0.0.1:5299' })
  const old = owner.local(document, first.revision).catch((error) => error)
  await started
  const remote = owner.call({ operation: 'begin', document, url: 'https://remote.example' })
  owner.call({ operation: 'commit', document, revision: remote.revision, session: { url: remote.url, token: 'jaz_remote', deviceId: 'identity', remembered: false } })
  const next = owner.call({ operation: 'begin', document, url: 'http://127.0.0.1:5299' })
  const resumed = owner.local(document, next.revision)
  expect(f.counts().starts).toBe(1)
  expect(f.counts().registrations).toBe(1)
  release(Response.json({ token: 'jaz_device-bearer' }))
  expect(await old).toBeInstanceOf(Error)
  const session = await resumed
  expect(f.counts().registrations).toBe(2)
  expect(owner.read().snapshot.phase).toBe('pending')
  owner.call({ operation: 'commit', document, revision: next.revision, session })
  expect(owner.read().snapshot.session.token).toBe('jaz_device-bearer')
})
