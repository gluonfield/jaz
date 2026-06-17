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

export type DeviceProfile = DeviceIdentity & DeviceMetadata

const DEVICE_IDENTITY_KEY = 'jaz.deviceIdentity'

export async function getDeviceIdentity(): Promise<DeviceIdentity> {
  if (window.jaz?.getDeviceIdentity) return window.jaz.getDeviceIdentity()
  return getBrowserDeviceIdentity()
}

export async function getDeviceProfile(): Promise<DeviceProfile> {
  const [identity, metadata] = await Promise.all([getDeviceIdentity(), getDeviceMetadata()])
  return { ...identity, ...metadata }
}

async function getDeviceMetadata(): Promise<DeviceMetadata> {
  if (window.jaz?.getDeviceMetadata) return window.jaz.getDeviceMetadata()
  const platform = navigator.platform?.trim() || 'Browser'
  return {
    name: defaultBrowserDeviceName(platform),
    platform,
    device_family: 'Browser',
    model_identifier: '',
    app_version: '',
  }
}

async function getBrowserDeviceIdentity(): Promise<DeviceIdentity> {
  const stored = readStoredBrowserIdentity()
  if (stored) return stored
  const bytes = new Uint8Array(32)
  crypto.getRandomValues(bytes)
  const identity = {
    device_id: await sha256Hex(bytes),
    public_key: base64Url(bytes),
  }
  localStorage.setItem(DEVICE_IDENTITY_KEY, JSON.stringify(identity))
  return identity
}

function readStoredBrowserIdentity(): DeviceIdentity | null {
  try {
    const parsed = JSON.parse(localStorage.getItem(DEVICE_IDENTITY_KEY) || '') as Partial<DeviceIdentity>
    if (typeof parsed.device_id === 'string' && typeof parsed.public_key === 'string') {
      return { device_id: parsed.device_id, public_key: parsed.public_key }
    }
  } catch {
    return null
  }
  return null
}

async function sha256Hex(bytes: Uint8Array): Promise<string> {
  const input = bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength) as ArrayBuffer
  const hash = new Uint8Array(await crypto.subtle.digest('SHA-256', input))
  return Array.from(hash, (byte) => byte.toString(16).padStart(2, '0')).join('')
}

function base64Url(bytes: Uint8Array): string {
  let binary = ''
  for (const byte of bytes) binary += String.fromCharCode(byte)
  return btoa(binary).replaceAll('+', '-').replaceAll('/', '_').replace(/=+$/g, '')
}

function defaultBrowserDeviceName(platform: string): string {
  return platform ? `Jaz browser on ${platform}` : 'Jaz browser'
}
