import { afterEach, expect, test } from 'bun:test'
import { build } from 'bun'
import { createCipheriv, createDecipheriv, pbkdf2Sync, randomBytes } from 'node:crypto'
import { EventEmitter } from 'node:events'
import { mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { createRequire } from 'node:module'
import { resolve, join } from 'node:path'
import { DatabaseSync } from 'node:sqlite'

const result = await build({
  entrypoints: ['password-fixture'],
  target: 'node',
  format: 'cjs',
  external: ['electron'],
  plugins: [{
    name: 'password-fixture',
    setup(builder) {
      builder.onResolve({ filter: /^password-fixture$/ }, () => ({ path: 'password-fixture', namespace: 'fixture' }))
      builder.onLoad({ filter: /.*/, namespace: 'fixture' }, () => ({
        contents: `export { installBrowserPasswords } from ${JSON.stringify(new URL('./browserPasswords.ts', import.meta.url).pathname)}
export { BrowserPasswordStore, getBrowserPasswordStore } from ${JSON.stringify(new URL('./browserPasswordStore.ts', import.meta.url).pathname)}
export { BrowserProfileImporter } from ${JSON.stringify(new URL('./browserProfileImport.ts', import.meta.url).pathname)}`,
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
const directories = []
afterEach(() => {
  for (const directory of directories.splice(0)) {
    rmSync(directory, { recursive: true, force: true })
  }
})

function fixture() {
  mkdirSync(resolve('../runs/safe-storage-dialog'), { recursive: true })
  const directory = mkdtempSync(resolve('../runs/safe-storage-dialog/password-test-'))
  directories.push(directory)
  const file = join(directory, 'browser-passwords.enc')
  const key = randomBytes(32)
  const calls = []
  let available = true
  let failWrite = false
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
  const handlers = new Map()
  const host = { id: 1, getType: () => 'window', getURL: () => 'file:///fixture/renderer/index.html', isDestroyed: () => false, send: () => {} }
  host.mainFrame = { url: host.getURL() }
  const partition = {}
  const sent = []
  const target = Object.assign(new EventEmitter(), {
    id: 2,
    getType: () => 'webview',
    getURL: () => target.mainFrame.url,
    isDestroyed: () => false,
    hostWebContents: host,
    session: partition,
    mainFrame: { url: 'https://login.test/form', send: (...args) => sent.push(args) },
  })
  const electron = {
    app: { getPath: () => directory },
    safeStorage: provider,
    session: { fromPartition: () => partition },
    webContents: { fromId: (id) => id === target.id ? target : undefined, getAllWebContents: () => [target] },
    ipcMain: {
      handle: (name, callback) => handlers.set(name, callback),
      on: (name, callback) => handlers.set(name, callback),
    },
  }
  const module = { exports: {} }
  evaluate((name) => name === 'electron' ? electron : require(name), module, module.exports)
  const { installBrowserPasswords, BrowserPasswordStore, getBrowserPasswordStore, BrowserProfileImporter } = module.exports
  installBrowserPasswords()
  const store = getBrowserPasswordStore()
  const origin = 'https://login.test'
  const credential = { origin, username: 'fixture-user', password: 'fixture-password' }
  const event = { sender: host, senderFrame: host.mainFrame }
  return {
    directory, file, calls, sent, target, credential, store, BrowserProfileImporter,
    seed: (records = [credential]) => writeFileSync(file, encrypt(JSON.stringify(records))),
    available: (value) => { available = value },
    failWrite: (value) => { failWrite = value },
    state: (action, request = event) => handlers.get('jaz:browser-passwords')(request, target.id, action),
    capture: (value = credential) => handlers.get('jaz:browser-passwords:capture')({ sender: target, senderFrame: target.mainFrame }, value),
    reopen: () => new BrowserPasswordStore(file),
  }
}

test('passive browser state, navigation, capture and dismissal never unlock a saved vault', () => {
  const f = fixture()
  f.seed()
  const before = readFileSync(f.file)
  f.available(false)
  expect(f.state()).toMatchObject({ locked: true, usernames: [] })
  f.target.emit('did-navigate', {}, 'https://login.test/other')
  f.capture()
  const state = f.state()
  expect(state.pending).toMatchObject({ username: f.credential.username, update: false })
  expect(JSON.stringify(state)).not.toContain(f.credential.password)
  expect(f.state({ kind: 'dismiss', id: state.pending.id }).pending).toBeUndefined()
  expect(f.calls).toEqual([])
  expect(readFileSync(f.file)).toEqual(before)
})

test('explicit unlock is one read; passive updates and filling reuse it without another unlock', () => {
  const f = fixture()
  f.seed()
  expect(f.state({ kind: 'unlock' })).toMatchObject({ locked: false, usernames: [f.credential.username] })
  expect(f.calls).toEqual(['available', 'decrypt'])
  f.calls.length = 0
  f.capture()
  expect(f.state().pending).toBeUndefined()
  f.state({ kind: 'fill', origin: f.credential.origin, username: f.credential.username })
  expect(f.sent.at(-1)).toEqual(['jaz:browser-passwords:fill', f.credential])
  expect(f.calls).toEqual([])
  expect(f.state({ kind: 'fill', origin: 'https://another.test', username: f.credential.username }).error).toBeDefined()
  f.target.mainFrame.url = 'https://another.test/'
  expect(f.state().usernames).toEqual([])
})

test('denied unlock and save preserve ciphertext and pending capture without retrying on refresh', () => {
  const f = fixture()
  f.seed()
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
  const f = fixture()
  f.seed()
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

test('untrusted frames and expired site actions cannot unlock or modify passwords', () => {
  const f = fixture()
  f.seed()
  for (const request of [
    { sender: f.target, senderFrame: f.target.mainFrame },
    { sender: f.target.hostWebContents, senderFrame: { url: 'file:///fixture/renderer/index.html' } },
  ]) {
    expect(() => f.state({ kind: 'unlock' }, request)).toThrow('owning Jaz')
  }
  f.capture()
  const id = f.state().pending.id
  f.target.mainFrame.url = 'https://another.test/'
  expect(f.state({ kind: 'save', id }).error).toContain('expired')
  f.target.mainFrame.url = 'http://login.test/'
  expect(f.state({ kind: 'unlock' }).error).toContain('HTTPS')
  expect(f.calls).toEqual([])
})

test('browser import updates the same unlocked vault used by password management', async () => {
  const f = fixture()
  f.seed()
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
  const importer = new f.BrowserProfileImporter(async () => [profile], async () => 'fixture-browser-key')
  expect(await importer.import(profile.id, { cookies: false, passwords: true })).toEqual({ passwords: { imported: 1, failed: 0, skipped: 0 } })
  expect(f.state().usernames).toEqual([f.credential.username, 'imported'])
  expect(f.calls).toEqual(['available', 'encrypt'])
  expect(f.reopen().read().map((record) => record.username)).toEqual([f.credential.username, 'imported'])
})
