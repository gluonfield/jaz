import type { FolderAccessMode, MachineFolderGrant } from './api/serverTypes'

const STORAGE_KEY_PREFIX = 'jaz.cloud.machineConfig.'
const CONTROL_CHAR_RE = /[\x00-\x1f\x7f]/

export type CommandShellKind = 'posix' | 'powershell'

export interface LocalMachineShareConfig {
  deviceId: string
  displayName: string
  allowExec: boolean
  grants: MachineFolderGrant[]
}

function normalizeOrigin(url: string): string {
  const trimmed = url.trim().replace(/\/+$/, '')
  if (!trimmed) return ''
  try {
    return new URL(/^https?:\/\//i.test(trimmed) ? trimmed : `https://${trimmed}`).origin
  } catch {
    return trimmed
  }
}

function storageKey(cloudUrl: string): string {
  const origin = normalizeOrigin(cloudUrl)
  return `${STORAGE_KEY_PREFIX}${origin || 'default'}`
}

function defaultDeviceId(): string {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return `desktop-${crypto.randomUUID().slice(0, 8)}`
  }
  const bytes = new Uint8Array(4)
  crypto.getRandomValues(bytes)
  return `desktop-${Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('')}`
}

function defaultConfig(): LocalMachineShareConfig {
  return {
    deviceId: defaultDeviceId(),
    displayName: '',
    allowExec: false,
    grants: [],
  }
}

export function defaultCommandShell(): CommandShellKind {
  if (typeof navigator !== 'undefined' && /Win/i.test(navigator.platform || navigator.userAgent || '')) {
    return 'powershell'
  }
  return 'posix'
}

export function validateGrantPath(value: string): string | null {
  const trimmed = value.trim()
  if (!trimmed) return 'Folder path is required'
  if (CONTROL_CHAR_RE.test(trimmed)) return 'Folder path cannot contain control or newline characters'
  return null
}

export function validateGrantId(value: string): string | null {
  const trimmed = value.trim()
  if (!trimmed) return 'Grant ID is required'
  if (CONTROL_CHAR_RE.test(trimmed) || /[=:]/.test(trimmed)) {
    return 'Grant ID cannot contain "=", ":", or control characters'
  }
  return null
}

export function loadLocalMachineConfig(cloudUrl = ''): LocalMachineShareConfig {
  try {
    const parsed = JSON.parse(localStorage.getItem(storageKey(cloudUrl)) ?? 'null') as Partial<LocalMachineShareConfig> | null
    if (!parsed || typeof parsed !== 'object') return defaultConfig()
    const grants = Array.isArray(parsed.grants)
      ? parsed.grants.filter(
          (g): g is MachineFolderGrant =>
            Boolean(
              g &&
              typeof g.id === 'string' &&
              typeof g.root_path === 'string' &&
              !validateGrantId(g.id) &&
              !validateGrantPath(g.root_path) &&
              (g.mode === 'ro' || g.mode === 'rw' || g.mode === 'rwd'),
            ),
        )
      : []
    const deviceId = typeof parsed.deviceId === 'string' && parsed.deviceId.trim() && !CONTROL_CHAR_RE.test(parsed.deviceId)
      ? parsed.deviceId.trim()
      : defaultDeviceId()
    const displayName = typeof parsed.displayName === 'string' && !CONTROL_CHAR_RE.test(parsed.displayName)
      ? parsed.displayName
      : ''
    return {
      deviceId,
      displayName,
      allowExec: parsed.allowExec === true,
      grants,
    }
  } catch {
    return defaultConfig()
  }
}

export function saveLocalMachineConfig(cloudUrl: string, config: LocalMachineShareConfig): void {
  try {
    localStorage.setItem(storageKey(cloudUrl), JSON.stringify(config))
  } catch {
    // Preference storage failure should not break Settings.
  }
}

export function deriveGrantId(rootPath: string, existing: MachineFolderGrant[]): string {
  const leaf = rootPath
    .trim()
    .replace(/[\\/]+$/, '')
    .split(/[\\/]/)
    .filter(Boolean)
    .at(-1) ?? 'folder'
  const base = leaf.toLowerCase().replace(/[^a-z0-9-]+/g, '-').replace(/^-+|-+$/g, '') || 'folder'
  const used = new Set(existing.map((grant) => grant.id))
  if (!used.has(base)) return base
  for (let i = 2; i < 1000; i += 1) {
    const candidate = `${base}-${i}`
    if (!used.has(candidate)) return candidate
  }
  return `${base}-${Date.now()}`
}

export const FOLDER_ACCESS_LABELS: Record<FolderAccessMode, string> = {
  ro: 'Read-only',
  rw: 'Read & write',
  rwd: 'Read, write & delete',
}

function quoteArg(value: string, shell: CommandShellKind): string {
  if (CONTROL_CHAR_RE.test(value)) {
    throw new Error('Command argument cannot contain control characters')
  }
  if (/^[a-zA-Z0-9._/:=@-]+$/.test(value)) return value
  if (shell === 'powershell') {
    return `'${value.replace(/'/g, "''")}'`
  }
  return `'${value.replace(/'/g, `'\\''`)}'`
}

// Generates a copyable `jaz connect-cloud` invocation. Intentionally omits `--token`
// because the CLI reads `JAZ_CLOUD_TOKEN` from the process environment without leaking it to argv.
export function buildConnectCloudCommand(input: {
  cloudUrl: string
  deviceId: string
  displayName: string
  grants: MachineFolderGrant[]
  allowExec: boolean
  shell?: CommandShellKind
}): string {
  const shell = input.shell ?? defaultCommandShell()
  const targetUrl = normalizeOrigin(input.cloudUrl) || input.cloudUrl.trim()
  if (!targetUrl || CONTROL_CHAR_RE.test(targetUrl)) {
    throw new Error('Invalid cloud URL')
  }
  const deviceId = input.deviceId.trim() || 'workstation-1'
  const parts = [
    'jaz connect-cloud',
    `--url ${quoteArg(targetUrl, shell)}`,
    `--device-id ${quoteArg(deviceId, shell)}`,
  ]
  const displayName = input.displayName.trim()
  if (displayName) {
    parts.push(`--name ${quoteArg(displayName, shell)}`)
  }
  for (const grant of input.grants) {
    const id = grant.id.trim()
    const path = grant.root_path.trim()
    if (!id && !path) continue
    const idErr = validateGrantId(id)
    if (idErr) throw new Error(idErr)
    const pathErr = validateGrantPath(path)
    if (pathErr) throw new Error(pathErr)
    parts.push(`--grant ${quoteArg(`${id}=${grant.mode}:${path}`, shell)}`)
  }
  if (input.allowExec) {
    parts.push('--allow-exec')
  }
  const continuation = shell === 'powershell' ? ' `\n  ' : ' \\\n  '
  return parts.join(continuation)
}
