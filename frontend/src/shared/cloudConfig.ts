import { validBackendOrigin } from './connectionSession'

declare const __JAZ_BUILD_CLOUD_URL__: string | undefined
declare const __JAZ_BUILD_LOCAL_ENABLED__: string | undefined

export type CloudConfig = {
  cloudUrl: string
  localEnabled: boolean
}

export function parseBoolFlag(value: string | boolean | undefined, fallback: boolean): boolean {
  if (typeof value === 'boolean') return value
  if (typeof value !== 'string') return fallback
  const normalized = value.trim().toLowerCase()
  if (['false', '0', 'no', 'off'].includes(normalized)) return false
  if (['true', '1', 'yes', 'on'].includes(normalized)) return true
  return fallback
}

export function normalizeCloudUrl(raw: string | undefined): string {
  const trimmed = raw?.trim().replace(/\/+$/, '') ?? ''
  if (!trimmed) return ''
  try {
    const origin = new URL(trimmed).origin
    return validBackendOrigin(origin) ? origin : ''
  } catch {
    return ''
  }
}

export function resolveCloudConfig(options: {
  runtimeUrl?: string
  buildUrl?: string
  runtimeLocalEnabled?: string | boolean
  buildLocalEnabled?: string | boolean
  defaultLocalEnabled: boolean
}): CloudConfig {
  const cloudUrl = normalizeCloudUrl(options.runtimeUrl) || normalizeCloudUrl(options.buildUrl)
  const buildLocal = parseBoolFlag(options.buildLocalEnabled, options.defaultLocalEnabled)
  const localEnabled = parseBoolFlag(options.runtimeLocalEnabled, buildLocal)
  return { cloudUrl, localEnabled }
}

export function readNodeCloudConfig(
  env: Record<string, string | undefined> = process.env,
  defaultLocalEnabled = true,
): CloudConfig {
  return resolveCloudConfig({
    runtimeUrl: env['JAZ_CLOUD_URL'],
    buildUrl: typeof __JAZ_BUILD_CLOUD_URL__ === 'string' ? __JAZ_BUILD_CLOUD_URL__ : undefined,
    runtimeLocalEnabled: env['JAZ_LOCAL_ENABLED'],
    buildLocalEnabled: typeof __JAZ_BUILD_LOCAL_ENABLED__ === 'string' ? __JAZ_BUILD_LOCAL_ENABLED__ : undefined,
    defaultLocalEnabled,
  })
}
