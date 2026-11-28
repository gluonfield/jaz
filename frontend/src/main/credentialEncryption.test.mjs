import { afterEach, expect, test } from 'bun:test'
import { build } from 'bun'
import { mkdirSync, mkdtempSync, rmSync } from 'node:fs'
import { createRequire } from 'node:module'
import { resolve } from 'node:path'

const result = await build({
  entrypoints: ['credential-fixture'],
  target: 'node',
  format: 'cjs',
  external: ['electron'],
  plugins: [{
    name: 'credential-fixture',
    setup(builder) {
      builder.onResolve({ filter: /^credential-fixture$/ }, () => ({ path: 'credential-fixture', namespace: 'fixture' }))
      builder.onLoad({ filter: /.*/, namespace: 'fixture' }, () => ({
        contents: `export { installCredentialEncryption } from ${JSON.stringify(new URL('./credentialEncryption.ts', import.meta.url).pathname)}
export { DesktopSecretStore } from ${JSON.stringify(new URL('./secretStore.ts', import.meta.url).pathname)}`,
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
const previousURL = process.env.ELECTRON_RENDERER_URL
const directories = []

afterEach(() => {
  if (previousURL === undefined) {
    delete process.env.ELECTRON_RENDERER_URL
  } else {
    process.env.ELECTRON_RENDERER_URL = previousURL
  }
  for (const dir of directories.splice(0)) {
    rmSync(dir, { recursive: true, force: true })
  }
})

function fixture({ platform = 'linux', isPackaged = false, available = true, trusted = true, child = false } = {}) {
  mkdirSync(resolve('../runs/secret-store-tests'), { recursive: true })
  const directory = mkdtempSync(resolve('../runs/secret-store-tests/cred-'))
  directories.push(directory)
  let handler
  let encryptionCalls = 0
  let decryptionCalls = 0
  const provider = {
    isEncryptionAvailable: () => available,
    getSelectedStorageBackend: () => 'test-keychain',
    encryptString(value) {
      encryptionCalls += 1
      return Buffer.from('sealed:' + value)
    },
    decryptString(value) {
      decryptionCalls += 1
      return value.toString().slice('sealed:'.length)
    },
  }
  const electron = {
    app: { isPackaged, getPath: () => directory },
    ipcMain: { on: (_name, callback) => { handler = callback } },
    safeStorage: provider,
  }
  const module = { exports: {} }
  evaluate((name) => name === 'electron' ? electron : require(name), module, module.exports)
  const store = new module.exports.DesktopSecretStore({
    platform,
    isPackaged,
    userDataDir: directory,
    safeStorage: provider,
  })
  module.exports.installCredentialEncryption(store)
  process.env.ELECTRON_RENDERER_URL = 'https://jaz.test'
  const frame = { url: trusted ? 'https://jaz.test/app' : 'https://untrusted.example' }
  const event = { sender: { getType: () => 'window', mainFrame: frame }, senderFrame: child ? { url: frame.url } : frame }
  return {
    call(operation, value, allowLegacy) {
      handler(event, operation, value, allowLegacy)
      return event.returnValue
    },
    calls: () => encryptionCalls,
    decryptCalls: () => decryptionCalls,
  }
}

test('connection credentials pass through the OS encryption boundary on non-macOS platforms', () => {
  const handler = fixture({ platform: 'linux' })
  const encrypted = handler.call('encrypt', 'device-token')
  expect(encrypted.error).toBeUndefined()
  expect(handler.calls()).toBe(1)
  expect(handler.call('decrypt', encrypted.value).value).toBe('device-token')
})

test('macOS dev credentials encrypt as enc:v2 without touching legacy safeStorage', () => {
  const handler = fixture({ platform: 'darwin', isPackaged: false, available: false })
  const encrypted = handler.call('encrypt', 'cloud-token')
  expect(encrypted.error).toBeUndefined()
  expect(encrypted.value.startsWith('enc:v2:')).toBe(true)
  expect(handler.call('decrypt', encrypted.value).value).toBe('cloud-token')
  expect(handler.calls()).toBe(0)
  expect(handler.decryptCalls()).toBe(0)
  const legacy = Buffer.from('sealed:old-token').toString('base64')
  expect(handler.call('decrypt', legacy, true).error).toContain('dev builds')
  expect(handler.decryptCalls()).toBe(0)
})

test('macOS packaged legacy enc:v1 decrypt requires explicit allowLegacy flag', () => {
  const handler = fixture({ platform: 'darwin', isPackaged: true, available: true })
  const legacy = Buffer.from('sealed:legacy-token').toString('base64')
  expect(handler.call('decrypt', legacy, false).error).toContain('explicit unlock')
  expect(handler.decryptCalls()).toBe(0)
  expect(handler.call('decrypt', legacy, true).value).toBe('legacy-token')
  expect(handler.decryptCalls()).toBe(1)
})

test.each([
  { trusted: false },
  { child: true },
  { available: false },
])('untrusted frames and unavailable keychains cannot save credentials: %j', (options) => {
  const handler = fixture({ platform: 'linux', ...options })
  expect(handler.call('encrypt', 'device-token').error).toBeDefined()
  expect(handler.calls()).toBe(0)
})
