import { createCipheriv, createDecipheriv, randomBytes } from 'node:crypto'
import { execFileSync } from 'node:child_process'
import { chmodSync, linkSync, lstatSync, mkdirSync, readFileSync, unlinkSync, writeFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { app, safeStorage } from 'electron'

export const ENC_V2_PREFIX = 'enc:v2:'
export const MAC_KEYCHAIN_ACCESS_GROUP = 'JDDZ55DT74.dev.wins.jaz'

const KEY_BYTES = 32
const NONCE_BYTES = 12
const TAG_BYTES = 16
const KEYCHAIN_TIMEOUT_MS = 5_000

export type SecretStoreErrorCode = 'keychain_unavailable' | 'dev_legacy_blocked' | 'legacy_locked'

export class SecretStoreError extends Error {
  constructor(readonly code: SecretStoreErrorCode, message: string) {
    super(message)
    this.name = 'SecretStoreError'
  }
}

export type KeychainHelperResult =
  | { status: 'ok'; key: string }
  | { status: 'not_found' }
  | { status: 'missing_entitlement' }
  | { status: 'error'; code?: number }

export type SafeStorageLike = Pick<
  typeof safeStorage,
  'isEncryptionAvailable' | 'getSelectedStorageBackend' | 'encryptString' | 'decryptString'
>

export type DesktopSecretStoreOptions = {
  platform?: NodeJS.Platform
  isPackaged?: boolean
  userDataDir?: string
  execPath?: string
  accessGroup?: string
  keychainHelper?: (command: 'get' | 'set', accessGroup: string, input?: string) => KeychainHelperResult
  safeStorage?: SafeStorageLike
}

export function isV2Ciphertext(value: string | Buffer): boolean {
  if (typeof value === 'string') {
    return value.startsWith(ENC_V2_PREFIX)
  }
  return value.subarray(0, ENC_V2_PREFIX.length).toString('utf8') === ENC_V2_PREFIX
}

export function sealV2(plaintext: string, key: Buffer): string {
  if (key.length !== KEY_BYTES) {
    throw new Error('Master key must be 32 bytes.')
  }
  const nonce = randomBytes(NONCE_BYTES)
  const cipher = createCipheriv('aes-256-gcm', key, nonce)
  const ciphertext = Buffer.concat([cipher.update(plaintext, 'utf8'), cipher.final()])
  const tag = cipher.getAuthTag()
  return ENC_V2_PREFIX + Buffer.concat([nonce, tag, ciphertext]).toString('base64')
}

export function openV2(payload: string, key: Buffer): string {
  if (key.length !== KEY_BYTES) {
    throw new Error('Master key must be 32 bytes.')
  }
  if (!payload.startsWith(ENC_V2_PREFIX)) {
    throw new Error('Invalid enc:v2 payload.')
  }
  const raw = Buffer.from(payload.slice(ENC_V2_PREFIX.length), 'base64')
  if (raw.length < NONCE_BYTES + TAG_BYTES) {
    throw new Error('Truncated enc:v2 payload.')
  }
  const nonce = raw.subarray(0, NONCE_BYTES)
  const tag = raw.subarray(NONCE_BYTES, NONCE_BYTES + TAG_BYTES)
  const ciphertext = raw.subarray(NONCE_BYTES + TAG_BYTES)
  const decipher = createDecipheriv('aes-256-gcm', key, nonce)
  decipher.setAuthTag(tag)
  return Buffer.concat([decipher.update(ciphertext), decipher.final()]).toString('utf8')
}

function runMacKeychainHelper(execPath: string, command: 'get' | 'set', accessGroup: string, input?: string): KeychainHelperResult {
  const helper = join(dirname(execPath), 'jaz-keychain')
  try {
    const stdout = execFileSync(helper, [command, accessGroup], {
      input,
      encoding: 'utf8',
      stdio: ['pipe', 'pipe', 'pipe'],
      timeout: KEYCHAIN_TIMEOUT_MS,
    }).trim()
    const parsed = JSON.parse(stdout) as Partial<KeychainHelperResult>
    if (parsed.status === 'ok' && typeof parsed.key === 'string') {
      return { status: 'ok', key: parsed.key }
    }
    if (parsed.status === 'not_found' || parsed.status === 'missing_entitlement') {
      return { status: parsed.status }
    }
    return { status: 'error', code: typeof (parsed as { code?: unknown }).code === 'number' ? (parsed as { code: number }).code : undefined }
  } catch (error) {
    const code = (error as NodeJS.ErrnoException)?.code
    return code === 'ENOENT' ? { status: 'missing_entitlement' } : { status: 'error' }
  }
}

function readKeyFile(file: string): Buffer | null {
  let stats
  try {
    stats = lstatSync(file)
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code === 'ENOENT') {
      return null
    }
    throw error
  }
  if (!stats.isFile() || stats.isSymbolicLink()) {
    throw new SecretStoreError('keychain_unavailable', 'Master key file must be a regular file.')
  }
  const data = readFileSync(file)
  if (data.length !== KEY_BYTES) {
    throw new SecretStoreError('keychain_unavailable', `Master key file must contain ${KEY_BYTES} bytes.`)
  }
  try {
    chmodSync(file, 0o600)
  } catch {
    // Ignore chmod failures on filesystems without POSIX modes.
  }
  return data
}

function readOrCreateKeyFile(file: string): Buffer {
  const existing = readKeyFile(file)
  if (existing) {
    return existing
  }
  const dir = dirname(file)
  mkdirSync(dir, { recursive: true })
  const generated = randomBytes(KEY_BYTES)
  const tmp = join(dir, `.master-key-${process.pid}-${randomBytes(6).toString('hex')}.tmp`)
  try {
    writeFileSync(tmp, generated, { flag: 'wx', mode: 0o600 })
    chmodSync(tmp, 0o600)
    try {
      linkSync(tmp, file)
      return generated
    } catch (error) {
      if ((error as NodeJS.ErrnoException).code === 'EEXIST') {
        generated.fill(0)
        const winner = readKeyFile(file)
        if (winner) {
          return winner
        }
      }
      throw error
    }
  } finally {
    try {
      unlinkSync(tmp)
    } catch {
      // Temp file already cleaned up or never created.
    }
  }
}

function decodeBase64Key(encoded: string): Buffer {
  const key = Buffer.from(encoded, 'base64')
  if (key.length !== KEY_BYTES) {
    throw new SecretStoreError('keychain_unavailable', `Master key must decode to ${KEY_BYTES} bytes.`)
  }
  return key
}

export class DesktopSecretStore {
  private readonly platform: NodeJS.Platform
  private readonly isPackaged: boolean
  private readonly userDataDir: () => string
  private readonly execPath: string
  private readonly accessGroup: string
  private readonly helper: (command: 'get' | 'set', accessGroup: string, input?: string) => KeychainHelperResult
  private readonly storage: SafeStorageLike
  private cachedMasterKey?: Buffer
  private legacyUnlocked = false

  constructor(options: DesktopSecretStoreOptions = {}) {
    this.platform = options.platform ?? process.platform
    this.isPackaged = options.isPackaged ?? app?.isPackaged ?? false
    this.userDataDir = () => options.userDataDir ?? app.getPath('userData')
    this.execPath = options.execPath ?? process.execPath
    this.accessGroup = options.accessGroup ?? MAC_KEYCHAIN_ACCESS_GROUP
    this.helper = options.keychainHelper ?? ((command, group, input) => runMacKeychainHelper(this.execPath, command, group, input))
    this.storage = options.safeStorage ?? safeStorage
  }

  get usesMasterKey(): boolean {
    return this.platform === 'darwin'
  }

  canAutoUnlock(raw?: Buffer): boolean {
    if (this.usesMasterKey) {
      return !raw || isV2Ciphertext(raw) || (this.isPackaged && this.legacyUnlocked)
    }
    return !raw || this.platform === 'win32'
  }

  encryptText(plaintext: string): string {
    if (this.usesMasterKey) {
      return sealV2(plaintext, this.masterKey())
    }
    this.requireSafeStorage()
    return this.storage.encryptString(plaintext).toString('base64')
  }

  encryptBuffer(plaintext: string): Buffer {
    if (this.usesMasterKey) {
      return Buffer.from(sealV2(plaintext, this.masterKey()), 'utf8')
    }
    this.requireSafeStorage()
    return this.storage.encryptString(plaintext)
  }

  decryptText(ciphertext: string, options?: { allowLegacy?: boolean }): { plaintext: string; migratedFromLegacy: boolean } {
    return isV2Ciphertext(ciphertext)
      ? { plaintext: openV2(ciphertext, this.masterKey()), migratedFromLegacy: false }
      : this.decryptLegacy(Buffer.from(ciphertext, 'base64'), options)
  }

  decryptBuffer(encrypted: Buffer, options?: { allowLegacy?: boolean }): { plaintext: string; migratedFromLegacy: boolean } {
    return isV2Ciphertext(encrypted)
      ? { plaintext: openV2(encrypted.toString('utf8'), this.masterKey()), migratedFromLegacy: false }
      : this.decryptLegacy(encrypted, options)
  }

  private decryptLegacy(encrypted: Buffer, options?: { allowLegacy?: boolean }): { plaintext: string; migratedFromLegacy: boolean } {
    if (this.usesMasterKey) {
      if (!this.isPackaged) {
        throw new SecretStoreError('dev_legacy_blocked', 'Legacy saved secrets cannot be unlocked in dev builds.')
      }
      if (!options?.allowLegacy && !this.legacyUnlocked) {
        throw new SecretStoreError('legacy_locked', 'Legacy saved secrets require explicit unlock.')
      }
    }
    this.requireSafeStorage()
    const plaintext = this.storage.decryptString(encrypted)
    this.legacyUnlocked = true
    return { plaintext, migratedFromLegacy: this.usesMasterKey }
  }

  private masterKey(): Buffer {
    if (this.cachedMasterKey) {
      return this.cachedMasterKey
    }
    if (!this.isPackaged) {
      return this.cachedMasterKey = readOrCreateKeyFile(join(this.userDataDir(), 'dev-master.key'))
    }
    const localFallbackFile = join(this.userDataDir(), 'master.key')
    const found = this.helper('get', this.accessGroup)
    if (found.status === 'ok') {
      return this.cachedMasterKey = decodeBase64Key(found.key)
    }
    if (found.status === 'missing_entitlement') {
      return this.cachedMasterKey = readOrCreateKeyFile(localFallbackFile)
    }
    if (found.status === 'not_found') {
      const existingLocal = readKeyFile(localFallbackFile)
      const candidate = existingLocal ?? randomBytes(KEY_BYTES)
      const stored = this.helper('set', this.accessGroup, candidate.toString('base64'))
      if (stored.status === 'ok') {
        const winningKey = decodeBase64Key(stored.key)
        if (existingLocal && winningKey.equals(existingLocal)) {
          try {
            unlinkSync(localFallbackFile)
          } catch {
            // Best-effort cleanup after confirming fallback key is persisted in Data Protection Keychain.
          }
        }
        candidate.fill(0)
        return this.cachedMasterKey = winningKey
      }
      candidate.fill(0)
      if (stored.status === 'missing_entitlement') {
        return this.cachedMasterKey = readOrCreateKeyFile(localFallbackFile)
      }
    }
    throw new SecretStoreError('keychain_unavailable', 'Saved secrets require an available system keychain.')
  }

  private requireSafeStorage(): void {
    if (!this.storage.isEncryptionAvailable() || (this.platform === 'linux' && this.storage.getSelectedStorageBackend() === 'basic_text')) {
      throw new SecretStoreError('keychain_unavailable', 'Saved passwords require an available system keychain.')
    }
  }
}

let sharedSecretStore: DesktopSecretStore | undefined

export function getDesktopSecretStore(): DesktopSecretStore {
  return sharedSecretStore ??= new DesktopSecretStore()
}
