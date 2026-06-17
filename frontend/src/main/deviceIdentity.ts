import { createHash, createPrivateKey, createPublicKey, generateKeyPairSync, sign, verify } from 'node:crypto'
import { execFileSync } from 'node:child_process'
import { mkdirSync, readFileSync, writeFileSync } from 'node:fs'
import { arch, hostname, platform } from 'node:os'
import { dirname, join } from 'node:path'
import { app } from 'electron'

export type DeviceIdentity = {
  device_id: string
  public_key: string
}

export type DeviceMetadata = {
  name: string
  platform: string
  device_family: string
  model_identifier: string
  app_version: string
}

type StoredIdentity = DeviceIdentity & {
  version: 1
  public_key_pem: string
  private_key_pem: string
  created_at_ms: number
}

const SPKI_PREFIX = Buffer.from('302a300506032b6570032100', 'hex')

export function getDeviceIdentity(): DeviceIdentity {
  const existing = readStoredIdentity()
  if (existing) return { device_id: existing.device_id, public_key: existing.public_key }
  const created = createIdentity()
  writeStoredIdentity(created)
  return { device_id: created.device_id, public_key: created.public_key }
}

export function getDeviceMetadata(): DeviceMetadata {
  const devicePlatform = platformLabel()
  const family = deviceFamily()
  return {
    name: defaultDeviceName(family),
    platform: devicePlatform,
    device_family: family,
    model_identifier: modelIdentifier(),
    app_version: app.getVersion(),
  }
}

function readStoredIdentity(): StoredIdentity | null {
  try {
    const parsed = JSON.parse(readFileSync(identityPath(), 'utf8')) as Partial<StoredIdentity>
    if (
      parsed.version !== 1 ||
      typeof parsed.public_key_pem !== 'string' ||
      typeof parsed.private_key_pem !== 'string'
    ) {
      return null
    }
    const identity = parsed as StoredIdentity
    const raw = rawPublicKey(identity.public_key_pem)
    const publicKey = base64Url(raw)
    const deviceID = deviceIDForRawKey(raw)
    if (identity.device_id !== deviceID || identity.public_key !== publicKey || !keyPairMatches(identity)) {
      return null
    }
    return identity
  } catch {
    return null
  }
}

function createIdentity(): StoredIdentity {
  const { publicKey, privateKey } = generateKeyPairSync('ed25519')
  const publicKeyPem = publicKey.export({ type: 'spki', format: 'pem' })
  const privateKeyPem = privateKey.export({ type: 'pkcs8', format: 'pem' })
  const raw = rawPublicKey(publicKeyPem)
  return {
    version: 1,
    device_id: deviceIDForRawKey(raw),
    public_key: base64Url(raw),
    public_key_pem: publicKeyPem,
    private_key_pem: privateKeyPem,
    created_at_ms: Date.now(),
  }
}

function writeStoredIdentity(identity: StoredIdentity): void {
  const file = identityPath()
  mkdirSync(dirname(file), { recursive: true, mode: 0o700 })
  writeFileSync(file, `${JSON.stringify(identity, null, 2)}\n`, { mode: 0o600 })
}

function identityPath(): string {
  return join(app.getPath('userData'), 'device-identity.json')
}

function rawPublicKey(publicKeyPem: string): Buffer {
  const key = createPublicKey(publicKeyPem)
  const spki = key.export({ type: 'spki', format: 'der' }) as Buffer
  if (spki.length === SPKI_PREFIX.length + 32 && spki.subarray(0, SPKI_PREFIX.length).equals(SPKI_PREFIX)) {
    return spki.subarray(SPKI_PREFIX.length)
  }
  return spki
}

function keyPairMatches(identity: Pick<StoredIdentity, 'public_key_pem' | 'private_key_pem'>): boolean {
  try {
    const payload = Buffer.from('jaz-device-identity-self-check')
    const signature = sign(null, payload, createPrivateKey(identity.private_key_pem))
    return verify(null, payload, createPublicKey(identity.public_key_pem), signature)
  } catch {
    return false
  }
}

function deviceIDForRawKey(raw: Buffer): string {
  return createHash('sha256').update(raw).digest('hex')
}

function base64Url(raw: Buffer): string {
  return raw.toString('base64').replaceAll('+', '-').replaceAll('/', '_').replace(/=+$/g, '')
}

function defaultDeviceName(family: string): string {
  const host = hostname().trim().replace(/\.local$/i, '').replaceAll('-', ' ')
  return host || `Jaz ${family}`
}

function platformLabel(): string {
  switch (platform()) {
    case 'darwin':
      return 'macOS'
    case 'win32':
      return 'Windows'
    case 'linux':
      return 'Linux'
    default:
      return platform()
  }
}

function deviceFamily(): string {
  switch (platform()) {
    case 'darwin':
      return 'Mac'
    case 'win32':
    case 'linux':
      return 'Desktop'
    default:
      return 'Desktop'
  }
}

function modelIdentifier(): string {
  if (platform() === 'darwin') {
    try {
      return execFileSync('/usr/sbin/sysctl', ['-n', 'hw.model'], {
        encoding: 'utf8',
        timeout: 500,
      }).trim()
    } catch {
      return arch()
    }
  }
  return arch()
}
