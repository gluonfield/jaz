import { afterEach, expect, test } from 'bun:test'
import { build } from 'bun'

const result = await build({
  entrypoints: [new URL('./credentialEncryption.ts', import.meta.url).pathname],
  target: 'node',
  format: 'cjs',
  external: ['electron'],
})
if (!result.success) {
  throw new AggregateError(result.logs)
}
const evaluate = new Function('require', 'module', 'exports', await result.outputs[0].text())
const previousURL = process.env.ELECTRON_RENDERER_URL
afterEach(() => {
  if (previousURL === undefined) {
    delete process.env.ELECTRON_RENDERER_URL
  } else {
    process.env.ELECTRON_RENDERER_URL = previousURL
  }
})

function fixture({ available = true, trusted = true, child = false } = {}) {
  let handler
  let encryptionCalls = 0
  const provider = {
    isEncryptionAvailable: () => available,
    getSelectedStorageBackend: () => 'test-keychain',
    encryptString(value) {
      encryptionCalls += 1
      return Buffer.from('sealed:' + value)
    },
    decryptString(value) {
      return value.toString().slice('sealed:'.length)
    },
  }
  const electron = { ipcMain: { on: (_name, callback) => { handler = callback } }, safeStorage: provider }
  const module = { exports: {} }
  evaluate(() => electron, module, module.exports)
  module.exports.installCredentialEncryption()
  process.env.ELECTRON_RENDERER_URL = 'https://jaz.test'
  const frame = { url: trusted ? 'https://jaz.test/app' : 'https://untrusted.example' }
  const event = { sender: { getType: () => 'window', mainFrame: frame }, senderFrame: child ? { url: frame.url } : frame }
  return {
    call(operation, value) {
      handler(event, operation, value)
      return event.returnValue
    },
    calls: () => encryptionCalls,
  }
}

test('connection credentials pass through the OS encryption boundary', () => {
  const handler = fixture()
  const encrypted = handler.call('encrypt', 'device-token')
  expect(encrypted.error).toBeUndefined()
  expect(handler.calls()).toBe(1)
  expect(handler.call('decrypt', encrypted.value).value).toBe('device-token')
})

test.each([
  { trusted: false },
  { child: true },
  { available: false },
])('untrusted frames and unavailable keychains cannot save credentials: %j', (options) => {
  const handler = fixture(options)
  expect(handler.call('encrypt', 'device-token').error).toBeDefined()
  expect(handler.calls()).toBe(0)
})
