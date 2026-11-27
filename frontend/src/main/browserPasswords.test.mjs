import { afterEach, expect, test } from 'bun:test'
import { build } from 'bun'
import { createCipheriv, createDecipheriv, pbkdf2Sync, randomBytes } from 'node:crypto'
import { EventEmitter } from 'node:events'
import { mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { createRequire } from 'node:module'
import { resolve, join } from 'node:path'
import { Database } from 'bun:sqlite'

const result = await build({
  entrypoints: ['password-fixture'],
  target: 'node',
  format: 'cjs',
  external: ['electron', 'node:sqlite'],
  plugins: [{
    name: 'password-fixture',
    setup(builder) {
      builder.onResolve({ filter: /^password-fixture$/ }, () => ({ path: 'password-fixture', namespace: 'fixture' }))
      builder.onLoad({ filter: /.*/, namespace: 'fixture' }, () => ({
        contents: `export { installBrowserPasswords } from ${JSON.stringify(new URL('./browserPasswords.ts', import.meta.url).pathname)}
export { BrowserPasswordStore, getBrowserPasswordStore } from ${JSON.stringify(new URL('./browserPasswordStore.ts', import.meta.url).pathname)}
export { DesktopSecretStore, ENC_V2_PREFIX } from ${JSON.stringify(new URL('./secretStore.ts', import.meta.url).pathname)}
export { BrowserProfileImporter } from ${JSON.stringify(new URL('./browserProfileImport.ts', import.meta.url).pathname)}
export { setTrustedRendererPath } from ${JSON.stringify(new URL('./permissions.ts', import.meta.url).pathname)}`,
        loader: 'js',
      }))
    },
  }],
})
if (!result.success) {
  throw new AggregateError(result.logs)
}
const evaluate = new Function('require', 'module', 'exports', await result.outputs[0].text())
const require = createRequire(import.meta.url)
class DatabaseSync extends Database {
  constructor(file, options) {
    super(file, options?.readOnly ? { readonly: true } : { readwrite: true, create: true })
  }

  prepare(sql) {
    const statement = super.prepare(sql)
    statement.setReadBigInts = (enabled) => statement.safeIntegers(enabled)
    return statement
  }
}

const directories = []
afterEach(() => {
  for (const directory of directories.splice(0)) {
    rmSync(directory, { recursive: true, force: true })
  }
})

function fixture({ platform = 'linux', isPackaged = false, startNow = 1_700_000_000_000 } = {}) {
  mkdirSync(resolve('../runs/safe-storage-dialog'), { recursive: true })
  const directory = mkdtempSync(resolve('../runs/safe-storage-dialog/password-test-'))
  directories.push(directory)
  const file = join(directory, 'browser-passwords.enc')
  const key = randomBytes(32)
  const calls = []
  let available = true
  let failWrite = false
  let clock = startNow
  let dpKey = null
  const encrypt = (value) => {
    const nonce = randomBytes(12)
    const cipher = createCipheriv('aes-256-gcm', key, nonce)
    return Buffer.concat([nonce, cipher.update(value), cipher.final(), cipher.getAuthTag()])
  }
  const provider = {
    isEncryptionAvailable() {
      calls.push('available')
      return available
    },
    getSelectedStorageBackend: () => 'fixture-keychain',
    encryptString(value) {
      calls.push('encrypt')
      if (failWrite) {
        throw new Error('Fixture write denied')
      }
      return encrypt(value)
    },
    decryptString(value) {
      calls.push('decrypt')
      const cipher = createDecipheriv('aes-256-gcm', key, value.subarray(0, 12))
      cipher.setAuthTag(value.subarray(-16))
      return Buffer.concat([cipher.update(value.subarray(12, -16)), cipher.final()]).toString()
    },
  }
  const keychainHelper = (command, _group, input) => {
    if (command === 'get') {
      return dpKey ? { status: 'ok', key: dpKey } : { status: 'not_found' }
    }
    dpKey = input
    return { status: 'ok', key: dpKey }
  }
  const handlers = new Map()
  const host = { id: 1, getType: () => 'window', getURL: () => 'file:///fixture/renderer/index.html', isDestroyed: () => false, send: () => {} }
  host.mainFrame = { url: host.getURL() }
  const partition = {}
  const sent = []
  const makeFrame = (url) => {
    const frame = { url, send: (...args) => sent.push({ frame, args }) }
    return frame
  }
  const mainFrame = makeFrame('https://login.test/form')
  mainFrame.framesInSubtree = [mainFrame]
  const target = Object.assign(new EventEmitter(), {
    id: 2,
    getType: () => 'webview',
    getURL: () => target.mainFrame.url,
    isDestroyed: () => false,
    hostWebContents: host,
    session: partition,
    mainFrame,
  })
  const electron = {
    app: { isPackaged, getPath: () => directory },
    safeStorage: provider,
    session: { fromPartition: () => partition },
    webContents: { fromId: (id) => id === target.id ? target : undefined, getAllWebContents: () => [target] },
    ipcMain: {
      handle: (name, callback) => handlers.set(name, callback),
      on: (name, callback) => handlers.set(name, callback),
    },
  }
  const module = { exports: {} }
  evaluate((name) => name === 'electron' ? electron : name === 'node:sqlite' ? { DatabaseSync } : require(name), module, module.exports)
  const { installBrowserPasswords, BrowserPasswordStore, DesktopSecretStore, ENC_V2_PREFIX, BrowserProfileImporter, setTrustedRendererPath } = module.exports
  setTrustedRendererPath('/fixture/renderer/index.html')
  const makeSecrets = () => new DesktopSecretStore({
    platform,
    isPackaged,
    userDataDir: directory,
    keychainHelper,
    safeStorage: provider,
  })
  const store = new BrowserPasswordStore(file, makeSecrets())
  installBrowserPasswords(store, { now: () => clock })
  const origin = 'https://login.test'
  const credential = { origin, username: 'fixture-user', password: 'fixture-password' }
  const event = { sender: host, senderFrame: host.mainFrame }
  return {
    directory, file, calls, sent, target, credential, store, BrowserProfileImporter, ENC_V2_PREFIX,
    makeFrame,
    advance: (ms) => { clock += ms },
    seedLegacy: (records = [credential]) => writeFileSync(file, encrypt(JSON.stringify(records))),
    available: (value) => { available = value },
    failWrite: (value) => { failWrite = value },
    state: (action, request = event) => handlers.get('jaz:browser-passwords')(request, target.id, action),
    capture: (value = credential, frame = target.mainFrame) => handlers.get('jaz:browser-passwords:capture')({ sender: target, senderFrame: frame }, value),
    username: (value, frame = target.mainFrame) => handlers.get('jaz:browser-passwords:username')({ sender: target, senderFrame: frame }, value),
    result: (payload, frame = target.mainFrame) => handlers.get('jaz:browser-passwords:result')({ sender: target, senderFrame: frame }, payload),
    reopen: () => new BrowserPasswordStore(file, makeSecrets()),
  }
}

test('passive browser state, navigation, capture and dismissal never unlock a legacy vault', () => {
  const f = fixture({ platform: 'linux' })
  f.seedLegacy()
  const before = readFileSync(f.file)
  f.available(false)
  expect(f.state()).toMatchObject({ locked: true, usernames: [] })
  f.target.emit('did-navigate', {}, 'https://login.test/other')
  f.capture()
  const state = f.state()
  expect(state.pending).toMatchObject({ origin: f.credential.origin, username: f.credential.username, update: false })
  expect(JSON.stringify(state)).not.toContain(f.credential.password)
  expect(f.state({ kind: 'dismiss', id: state.pending.id }).pending).toBeUndefined()
  expect(f.calls).toEqual([])
  expect(readFileSync(f.file)).toEqual(before)
})

test('macOS enc:v2 vault auto-unlocks silently and packaged builds migrate legacy v1 only on explicit unlock', () => {
  const f = fixture({ platform: 'darwin', isPackaged: true })
  f.seedLegacy()
  expect(f.state()).toMatchObject({ locked: true, usernames: [] })
  expect(f.calls).toEqual([])

  expect(f.state({ kind: 'unlock' })).toMatchObject({ locked: false, usernames: [f.credential.username] })
  expect(f.calls).toEqual(['available', 'decrypt'])
  expect(readFileSync(f.file, 'utf8').startsWith(f.ENC_V2_PREFIX)).toBe(true)

  f.calls.length = 0
  const reopened = f.reopen()
  expect(reopened.tryAutoUnlock()).toBe(true)
  expect(reopened.cached).toEqual([f.credential])
  expect(f.calls).toEqual([])
})

test('macOS dev mode saves and auto-unlocks enc:v2 without ever calling safeStorage', () => {
  const f = fixture({ platform: 'darwin', isPackaged: false })
  f.available(false)
  expect(f.state()).toMatchObject({ locked: false, usernames: [] })
  f.capture()
  const id = f.state().pending.id
  expect(f.state({ kind: 'save', id })).toMatchObject({ locked: false, usernames: [f.credential.username] })
  expect(readFileSync(f.file, 'utf8').startsWith(f.ENC_V2_PREFIX)).toBe(true)
  expect(f.calls).toEqual([])
  const reopened = f.reopen()
  expect(reopened.tryAutoUnlock()).toBe(true)
  expect(reopened.cached).toEqual([f.credential])
  expect(f.calls).toEqual([])
})

test('explicit unlock is one read; passive updates and filling reuse it without another unlock', () => {
  const f = fixture({ platform: 'linux' })
  f.seedLegacy()
  expect(f.state({ kind: 'unlock' })).toMatchObject({ locked: false, usernames: [f.credential.username] })
  expect(f.calls).toEqual(['available', 'decrypt'])
  f.calls.length = 0
  f.capture()
  expect(f.state().pending).toBeUndefined()
  f.state({ kind: 'fill', origin: f.credential.origin, username: f.credential.username })
  expect(f.sent.at(-1).args[0]).toBe('jaz:browser-passwords:fill')
  expect(f.sent.at(-1).args[1]).toMatchObject(f.credential)
  expect(f.calls).toEqual([])
  expect(f.state({ kind: 'fill', origin: 'https://another.test', username: f.credential.username }).error).toBeDefined()
  f.target.mainFrame.url = 'https://another.test/'
  expect(f.state().usernames).toEqual([])
})

test('pending credentials survive cross-origin redirects for 60s and save under the login origin', () => {
  const f = fixture({ platform: 'darwin', isPackaged: false })
  f.target.mainFrame.url = 'https://accounts.example.test/login'
  f.capture({ origin: 'https://accounts.example.test', username: 'alice@example.test', password: 'secret-password' })
  const pendingId = f.state().pending.id

  f.target.mainFrame.url = 'https://app.example.test/dashboard'
  f.target.emit('did-navigate', {}, 'https://app.example.test/dashboard')
  const redirected = f.state()
  expect(redirected.pending).toMatchObject({
    id: pendingId,
    origin: 'https://accounts.example.test',
    username: 'alice@example.test',
    update: false,
  })

  f.advance(59_000)
  const saved = f.state({ kind: 'save', id: pendingId })
  expect(saved.error).toBeUndefined()
  expect(saved.pending).toBeUndefined()
  expect(f.reopen().read()).toEqual([
    { origin: 'https://accounts.example.test', username: 'alice@example.test', password: 'secret-password' },
  ])

  f.target.mainFrame.url = 'https://accounts.example.test/login'
  f.capture({ origin: 'https://accounts.example.test', username: 'bob@example.test', password: 'second-password' })
  const expiredId = f.state().pending.id
  f.advance(60_001)
  expect(f.state().pending).toBeUndefined()
  expect(f.state({ kind: 'save', id: expiredId }).error).toContain('expired')
})

test('loopback localhost origins support capture, save, and fill while non-loopback HTTP is rejected', () => {
  const f = fixture({ platform: 'darwin', isPackaged: false })
  f.target.mainFrame.url = 'http://localhost:3000/login'
  f.capture({ origin: 'http://localhost:3000', username: 'dev@local', password: 'dev-password' })
  const id = f.state().pending.id
  expect(f.state({ kind: 'save', id }).usernames).toEqual(['dev@local'])
  expect(f.reopen().read()).toEqual([{ origin: 'http://localhost:3000', username: 'dev@local', password: 'dev-password' }])

  f.target.mainFrame.url = 'http://127.0.0.1:8080/signin'
  f.capture({ origin: 'http://127.0.0.1:8080', username: 'loop@local', password: 'loop-password' })
  expect(f.state().pending).toMatchObject({ origin: 'http://127.0.0.1:8080', username: 'loop@local' })

  f.target.mainFrame.url = 'http://insecure.example.test/login'
  expect(f.state({ kind: 'unlock' }).error).toContain('HTTPS')
})

test('embedded login iframe captures, fills only its matching frame, and aggregates frame results', () => {
  const f = fixture({ platform: 'darwin', isPackaged: false })
  f.target.mainFrame.url = 'https://app.example.test/'
  const iframe = f.makeFrame('https://auth.example.test/embed')
  f.target.mainFrame.framesInSubtree = [f.target.mainFrame, iframe]

  f.capture({ origin: 'https://auth.example.test', username: 'iframe-user', password: 'iframe-password' }, iframe)
  const pending = f.state().pending
  expect(pending).toMatchObject({ origin: 'https://auth.example.test', username: 'iframe-user' })
  f.state({ kind: 'save', id: pending.id })
  expect(f.state().entries).toEqual([{ origin: 'https://auth.example.test', username: 'iframe-user' }])

  f.sent.length = 0
  f.state({ kind: 'fill', origin: 'https://auth.example.test', username: 'iframe-user' })
  expect(f.sent.length).toBe(1)
  expect(f.sent[0].frame).toBe(iframe)
  expect(f.sent[0].args[1]).toMatchObject({ origin: 'https://auth.example.test', username: 'iframe-user', password: 'iframe-password' })

  f.sent.length = 0
  f.state({ kind: 'capture' })
  expect(f.sent.length).toBe(2)
  const requestId = f.sent[0].args[1].requestId
  f.result({ requestId, found: false }, f.target.mainFrame)
  expect(f.state().error).toBeUndefined()
  f.result({ requestId, found: true }, iframe)
  expect(f.state().error).toBeUndefined()
})

test('two-step login preserves username across same-origin navigation', () => {
  const f = fixture({ platform: 'darwin', isPackaged: false })
  f.target.mainFrame.url = 'https://sso.example.test/step1'
  f.username({ origin: 'https://sso.example.test', username: 'step1@example.test' })
  f.target.mainFrame.url = 'https://sso.example.test/step2'
  f.target.emit('did-navigate', {}, 'https://sso.example.test/step2')
  f.capture({ origin: 'https://sso.example.test', username: '', password: 'step2-password' })
  expect(f.state().pending).toMatchObject({ origin: 'https://sso.example.test', username: 'step1@example.test' })
})

test('denied unlock and save preserve ciphertext and pending capture without retrying on refresh', () => {
  const f = fixture({ platform: 'linux' })
  f.seedLegacy()
  const before = readFileSync(f.file)
  f.available(false)
  f.capture()
  const id = f.state().pending.id
  expect(f.state({ kind: 'unlock' }).error).toContain('keychain')
  expect(f.calls).toEqual(['available'])
  f.calls.length = 0
  expect(f.state({ kind: 'save', id }).error).toContain('keychain')
  expect(f.calls).toEqual(['available'])
  f.calls.length = 0
  expect(f.state().pending.id).toBe(id)
  f.state()
  expect(f.calls).toEqual([])
  expect(readFileSync(f.file)).toEqual(before)
  f.available(true)
  expect(f.state({ kind: 'unlock' }).usernames).toEqual([f.credential.username])
})

test('save, import and remove publish cached changes only after encrypted persistence succeeds', () => {
  const f = fixture({ platform: 'linux' })
  f.seedLegacy()
  f.state({ kind: 'unlock' })
  const before = readFileSync(f.file)
  f.failWrite(true)
  f.capture({ ...f.credential, password: 'new-password' })
  expect(f.state({ kind: 'save', id: f.state().pending.id }).error).toBeDefined()
  expect(() => f.store.import([{ ...f.credential, username: 'imported' }])).toThrow()
  expect(f.store.cached).toEqual([f.credential])
  expect(readFileSync(f.file)).toEqual(before)
  f.failWrite(false)
  f.state({ kind: 'save', id: f.state().pending.id })
  expect(f.reopen().read()[0].password).toBe('new-password')
  expect(readFileSync(f.file).includes(Buffer.from('new-password'))).toBe(false)
  expect(f.store.import([{ ...f.credential, username: 'imported' }])).toBe(1)
  expect(f.state().usernames).toEqual([f.credential.username, 'imported'])
  f.state({ kind: 'remove', origin: f.credential.origin, username: 'imported' })
  expect(f.reopen().read().map((entry) => entry.username)).toEqual([f.credential.username])
})

test('untrusted frames cannot unlock or modify passwords', () => {
  const f = fixture({ platform: 'linux' })
  f.seedLegacy()
  for (const request of [
    { sender: f.target, senderFrame: f.target.mainFrame },
    { sender: f.target.hostWebContents, senderFrame: { url: 'file:///fixture/renderer/index.html' } },
  ]) {
    expect(() => f.state({ kind: 'unlock' }, request)).toThrow('owning Jaz')
  }
  expect(f.calls).toEqual([])
})

test('browser import updates the same unlocked vault used by password management', async () => {
  const f = fixture({ platform: 'linux' })
  f.seedLegacy()
  f.state({ kind: 'unlock' })
  f.calls.length = 0
  const database = join(f.directory, 'Login Data')
  const db = new DatabaseSync(database)
  const cipher = createCipheriv('aes-128-cbc', pbkdf2Sync('fixture-browser-key', 'saltysalt', 1003, 16, 'sha1'), Buffer.alloc(16, 32))
  const encrypted = Buffer.concat([Buffer.from('v10'), cipher.update('imported-password'), cipher.final()])
  try {
    db.exec('CREATE TABLE logins (origin_url TEXT, signon_realm TEXT, username_value TEXT, password_value BLOB, date_password_modified INTEGER, blacklisted_by_user INTEGER, scheme INTEGER)')
    db.prepare('INSERT INTO logins VALUES (?, ?, ?, ?, ?, ?, ?)').run(f.credential.origin + '/login', f.credential.origin + '/', 'imported', encrypted, 1, 0, 0)
  } finally {
    db.close()
  }
  const profile = { id: 'fixture-profile', passwordDatabases: [database], keychainService: 'Fixture Safe Storage' }
  const importer = new f.BrowserProfileImporter(async () => [profile], async () => 'fixture-browser-key', f.store)
  expect(await importer.import(profile.id, { cookies: false, passwords: true })).toEqual({ passwords: { imported: 1, failed: 0, skipped: 0 } })
  expect(f.state().usernames).toEqual([f.credential.username, 'imported'])
  expect(f.calls).toEqual(['available', 'encrypt'])
  expect(f.reopen().read().map((record) => record.username)).toEqual([f.credential.username, 'imported'])
})
