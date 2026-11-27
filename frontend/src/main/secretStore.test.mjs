import { afterEach, expect, test } from 'bun:test'
import { build } from 'bun'
import { execFileSync } from 'node:child_process'
import { randomBytes } from 'node:crypto'
import { existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, statSync, symlinkSync, writeFileSync } from 'node:fs'
import { createRequire } from 'node:module'
import { join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { prepareMacSigning } from '../../scripts/sign-mac.mjs'

const result = await build({
  entrypoints: [new URL('./secretStore.ts', import.meta.url).pathname],
  target: 'node',
  format: 'cjs',
  external: ['electron'],
})
if (!result.success) {
  throw new AggregateError(result.logs)
}
const evaluate = new Function('require', 'module', 'exports', await result.outputs[0].text())
const require = createRequire(import.meta.url)
const module = { exports: {} }
evaluate((name) => name === 'electron' ? { app: { isPackaged: false, getPath: () => '' }, safeStorage: {} } : require(name), module, module.exports)
const { DesktopSecretStore, ENC_V2_PREFIX, MAC_KEYCHAIN_ACCESS_GROUP, openV2, sealV2 } = module.exports

const directories = []
afterEach(() => {
  for (const dir of directories.splice(0)) {
    rmSync(dir, { recursive: true, force: true })
  }
})

function tempDir(prefix) {
  mkdirSync(resolve('../runs/secret-store-tests'), { recursive: true })
  const dir = mkdtempSync(resolve(`../runs/secret-store-tests/${prefix}-`))
  directories.push(dir)
  return dir
}

function forbiddenSafeStorage(calls) {
  return {
    isEncryptionAvailable() {
      calls.push('available')
      return false
    },
    getSelectedStorageBackend: () => 'basic_text',
    encryptString() {
      calls.push('encrypt')
      throw new Error('Must not call legacy safeStorage')
    },
    decryptString() {
      calls.push('decrypt')
      throw new Error('Must not call legacy safeStorage')
    },
  }
}

test('enc:v2 AES-256-GCM round-trips and detects tampering', () => {
  const key = randomBytes(32)
  const sealed = sealV2('{"secret":"value"}', key)
  expect(sealed.startsWith(ENC_V2_PREFIX)).toBe(true)
  expect(sealed).not.toContain('secret')
  expect(openV2(sealed, key)).toBe('{"secret":"value"}')

  const raw = Buffer.from(sealed.slice(ENC_V2_PREFIX.length), 'base64')
  raw[raw.length - 1] ^= 0xff
  const tampered = ENC_V2_PREFIX + raw.toString('base64')
  expect(() => openV2(tampered, key)).toThrow()
  expect(() => openV2(sealed, randomBytes(32))).toThrow()
})

test('macOS dev mode stores a 0600 dev-master.key in userData without touching Keychain or safeStorage', () => {
  const dir = tempDir('dev-iso')
  const safeCalls = []
  const helperCalls = []
  const store = new DesktopSecretStore({
    platform: 'darwin',
    isPackaged: false,
    userDataDir: dir,
    keychainHelper: (command) => {
      helperCalls.push(command)
      return { status: 'error' }
    },
    safeStorage: forbiddenSafeStorage(safeCalls),
  })

  const ciphertext = store.encryptText('dev-secret')
  expect(ciphertext.startsWith(ENC_V2_PREFIX)).toBe(true)
  expect(safeCalls).toEqual([])
  expect(helperCalls).toEqual([])

  const keyFile = join(dir, 'dev-master.key')
  expect(existsSync(keyFile)).toBe(true)
  expect(readFileSync(keyFile).length).toBe(32)
  expect(statSync(keyFile).mode & 0o777).toBe(0o600)

  const reopened = new DesktopSecretStore({
    platform: 'darwin',
    isPackaged: false,
    userDataDir: dir,
    safeStorage: forbiddenSafeStorage(safeCalls),
  })
  expect(reopened.decryptText(ciphertext)).toEqual({ plaintext: 'dev-secret', migratedFromLegacy: false })
  expect(safeCalls).toEqual([])
})

test('macOS packaged mode uses Data Protection Keychain helper with JDDZ55DT74.dev.wins.jaz', () => {
  const dir = tempDir('dp-keychain')
  const safeCalls = []
  const helperCalls = []
  let storedKey = null
  const keychainHelper = (command, accessGroup, input) => {
    helperCalls.push({ command, accessGroup })
    if (command === 'get') {
      return storedKey ? { status: 'ok', key: storedKey } : { status: 'not_found' }
    }
    storedKey = input
    return { status: 'ok', key: storedKey }
  }

  const first = new DesktopSecretStore({
    platform: 'darwin',
    isPackaged: true,
    userDataDir: dir,
    keychainHelper,
    safeStorage: forbiddenSafeStorage(safeCalls),
  })
  const encrypted = first.encryptBuffer('vault-payload')
  expect(encrypted.toString('utf8').startsWith(ENC_V2_PREFIX)).toBe(true)
  expect(helperCalls).toEqual([
    { command: 'get', accessGroup: MAC_KEYCHAIN_ACCESS_GROUP },
    { command: 'set', accessGroup: MAC_KEYCHAIN_ACCESS_GROUP },
  ])
  expect(safeCalls).toEqual([])
  expect(existsSync(join(dir, 'master.key'))).toBe(false)

  const second = new DesktopSecretStore({
    platform: 'darwin',
    isPackaged: true,
    userDataDir: dir,
    keychainHelper,
    safeStorage: forbiddenSafeStorage(safeCalls),
  })
  expect(second.decryptBuffer(encrypted)).toEqual({ plaintext: 'vault-payload', migratedFromLegacy: false })
  expect(safeCalls).toEqual([])
})

test('non-provisioned macOS signed runs fall back to 0600 master.key and promote it once provisioned', () => {
  const dir = tempDir('non-provisioned')
  const safeCalls = []
  const unprovisioned = new DesktopSecretStore({
    platform: 'darwin',
    isPackaged: true,
    userDataDir: dir,
    keychainHelper: () => ({ status: 'missing_entitlement' }),
    safeStorage: forbiddenSafeStorage(safeCalls),
  })
  const sealed = unprovisioned.encryptText('remembered-token')
  const fallbackFile = join(dir, 'master.key')
  expect(existsSync(fallbackFile)).toBe(true)
  expect(statSync(fallbackFile).mode & 0o777).toBe(0o600)
  expect(safeCalls).toEqual([])

  let dpKey = null
  const provisioned = new DesktopSecretStore({
    platform: 'darwin',
    isPackaged: true,
    userDataDir: dir,
    keychainHelper: (command, _group, input) => {
      if (command === 'get') {
        return dpKey ? { status: 'ok', key: dpKey } : { status: 'not_found' }
      }
      dpKey = input
      return { status: 'ok', key: dpKey }
    },
    safeStorage: forbiddenSafeStorage(safeCalls),
  })
  expect(provisioned.decryptText(sealed)).toEqual({ plaintext: 'remembered-token', migratedFromLegacy: false })
  expect(dpKey).toBeTruthy()
  expect(existsSync(fallbackFile)).toBe(false)
  expect(safeCalls).toEqual([])
})

test('key file loader rejects symlinks and promotion preserves master.key if keychain returns a different key', () => {
  const dir = tempDir('symlink-and-race')
  const external = join(dir, 'external.key')
  writeFileSync(external, randomBytes(32), { mode: 0o600 })
  symlinkSync(external, join(dir, 'dev-master.key'))

  const devStore = new DesktopSecretStore({
    platform: 'darwin',
    isPackaged: false,
    userDataDir: dir,
    safeStorage: forbiddenSafeStorage([]),
  })
  expect(() => devStore.encryptText('secret')).toThrow('regular file')

  const raceDir = tempDir('promotion-race')
  const localKey = randomBytes(32)
  const fallbackFile = join(raceDir, 'master.key')
  writeFileSync(fallbackFile, localKey, { mode: 0o600 })
  const otherKey = randomBytes(32).toString('base64')
  const racingStore = new DesktopSecretStore({
    platform: 'darwin',
    isPackaged: true,
    userDataDir: raceDir,
    keychainHelper: (command) => command === 'get' ? { status: 'not_found' } : { status: 'ok', key: otherKey },
    safeStorage: forbiddenSafeStorage([]),
  })
  const ciphertext = racingStore.encryptText('after-race')
  expect(openV2(ciphertext, Buffer.from(otherKey, 'base64'))).toBe('after-race')
  expect(existsSync(fallbackFile)).toBe(true)
})

const macOnly = process.platform === 'darwin' ? test : test.skip
const frontendDir = fileURLToPath(new URL('../../', import.meta.url))

macOnly('sign-mac leaves base entitlements untouched without a profile and activates keychain-access-groups with one', () => {
  const baseHelper = { entitlements: resolve(frontendDir, 'build/entitlements.mac.plist'), hardenedRuntime: true, additionalArguments: [] }
  const packager = { projectDir: frontendDir, appInfo: { id: 'dev.wins.jaz' } }

  const withoutProfile = prepareMacSigning(
    { app: '/fixture/Jaz.app', optionsForFile: () => baseHelper },
    packager,
    execFileSync,
    {},
  )
  expect(withoutProfile.optionsForFile('/fixture/Jaz.app').entitlements).toBe(baseHelper.entitlements)
  expect(withoutProfile.optionsForFile('/fixture/Jaz.app/Contents/MacOS/jaz-notch').additionalArguments).toContain('dev.wins.jaz')
  expect(withoutProfile.optionsForFile('/fixture/Jaz.app/Contents/MacOS/jaz-keychain').additionalArguments).toContain('dev.wins.jaz')

  const profile = {
    ExpirationDate: '2099-01-01T00:00:00Z',
    Entitlements: {
      'com.apple.application-identifier': 'JDDZ55DT74.dev.wins.jaz',
      'com.apple.developer.team-identifier': 'JDDZ55DT74',
      'keychain-access-groups': ['JDDZ55DT74.*'],
    },
  }
  const fakeExecute = (file, args, options) => {
    if (file !== '/usr/bin/security') return execFileSync(file, args, options)
    const xml = execFileSync('/usr/bin/plutil', ['-convert', 'xml1', '-o', '-', '-'], { ...options, input: JSON.stringify(profile) })
    const dated = execFileSync('/usr/bin/plutil', ['-replace', 'ExpirationDate', '-date', profile.ExpirationDate, '-o', '-', '-'], { ...options, input: xml })
    return execFileSync('/usr/bin/plutil', ['-insert', 'DeveloperCertificates', '-data', 'AA==', '-o', '-', '-'], { ...options, input: dated })
  }
  const withProfile = prepareMacSigning(
    { app: '/fixture/Jaz.app', provisioningProfile: '/fixture/jaz.provisionprofile', optionsForFile: () => baseHelper },
    packager,
    fakeExecute,
    {},
  )
  const appEntitlements = JSON.parse(execFileSync('/usr/bin/plutil', ['-convert', 'json', '-o', '-', withProfile.optionsForFile('/fixture/Jaz.app').entitlements], { encoding: 'utf8' }))
  expect(appEntitlements).toMatchObject({
    'com.apple.security.cs.allow-jit': true,
    'com.apple.application-identifier': 'JDDZ55DT74.dev.wins.jaz',
    'com.apple.developer.team-identifier': 'JDDZ55DT74',
    'keychain-access-groups': ['JDDZ55DT74.dev.wins.jaz'],
  })
  expect(withProfile.optionsForFile('/fixture/Jaz.app/Contents/MacOS/jaz-keychain').entitlements)
    .toBe(withProfile.optionsForFile('/fixture/Jaz.app').entitlements)
})
