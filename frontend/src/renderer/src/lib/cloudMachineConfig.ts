import type { FolderAccessMode, MachineFolderGrant } from './api/cloud'

const STORAGE_KEY = 'jaz.cloud.machineConfig'

export interface LocalMachineShareConfig {
  deviceId: string
  displayName: string
  allowExec: boolean
  grants: MachineFolderGrant[]
}

function defaultDeviceId(): string {
  return `desktop-${Math.random().toString(36).slice(2, 10)}`
}

function defaultConfig(): LocalMachineShareConfig {
  return {
    deviceId: defaultDeviceId(),
    displayName: '',
    allowExec: false,
    grants: [],
  }
}

export function loadLocalMachineConfig(): LocalMachineShareConfig {
  try {
    const parsed = JSON.parse(localStorage.getItem(STORAGE_KEY) ?? 'null') as Partial<LocalMachineShareConfig> | null
    if (!parsed || typeof parsed !== 'object') return defaultConfig()
    const grants = Array.isArray(parsed.grants)
      ? parsed.grants.filter(
          (g): g is MachineFolderGrant =>
            Boolean(g && typeof g.id === 'string' && typeof g.root_path === 'string' && (g.mode === 'ro' || g.mode === 'rw' || g.mode === 'rwd')),
        )
      : []
    return {
      deviceId: typeof parsed.deviceId === 'string' && parsed.deviceId.trim() ? parsed.deviceId.trim() : defaultDeviceId(),
      displayName: typeof parsed.displayName === 'string' ? parsed.displayName : '',
      allowExec: parsed.allowExec === true,
      grants,
    }
  } catch {
    return defaultConfig()
  }
}

export function saveLocalMachineConfig(config: LocalMachineShareConfig): void {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(config))
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
