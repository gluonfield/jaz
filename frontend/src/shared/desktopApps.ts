export const DESKTOP_APP_PERMISSIONS = ['notifications', 'always_on_top', 'shortcuts', 'tray', 'fs', 'full_access'] as const

export type DesktopAppPermission = (typeof DESKTOP_APP_PERMISSIONS)[number]

export interface DesktopAppIconSpec {
  kind?: string
  value?: string
  color?: string
  background?: string
}

export interface InstalledDesktopApp {
  appId: string
  workspaceId: string
  origin?: string
  title: string
  description?: string
  kind: 'artifact' | 'document'
  icon?: DesktopAppIconSpec
  installedVersion: number
  approvedBundleSha256: string
  latestBundleSha256: string
  declaredCapabilities: string[]
  permissions: DesktopAppPermission[]
  scopedFolders: string[]
  permissionsPaused: boolean
  isExternalOrg: boolean
  launcherPath?: string
  // The CDHash of a macOS launcher's shim loader, which a connecting shim must match.
  codeHash?: string
  shortcutAccelerator?: string
  installedAt: string
  updatedAt: string
}

export interface InstallDesktopAppInput {
  appId: string
  workspaceId: string
  origin?: string
  title: string
  description?: string
  kind: 'artifact' | 'document'
  icon?: DesktopAppIconSpec
  // The launcher icon as a square PNG data URL.
  iconPng: string
  version: number
  bundleSha256: string
  declaredCapabilities: string[]
  permissions?: DesktopAppPermission[]
  scopedFolders?: string[]
  isExternalOrg?: boolean
  allowDesktopFullAccess?: boolean
}

export interface DesktopAppFSEntry {
  name: string
  isDir: boolean
  size: number
}

export const DESKTOP_APPS_CHANNEL = 'jaz:desktop-apps:'

export interface DesktopAppsAPI {
  list: () => Promise<InstalledDesktopApp[]>
  get: (appId: string) => Promise<InstalledDesktopApp | null>
  install: (input: InstallDesktopAppInput) => Promise<InstalledDesktopApp>
  uninstall: (appId: string) => Promise<{ ok: boolean }>
  open: (appId: string) => Promise<void>
  reveal: (appId: string) => Promise<void>
  setPermissions: (
    appId: string,
    bundleSha256: string,
    permissions: DesktopAppPermission[],
    scopedFolders?: string[],
    allowDesktopFullAccess?: boolean,
  ) => Promise<InstalledDesktopApp>
  syncVersion: (
    appId: string,
    version: number,
    bundleSha256: string,
    declaredCapabilities?: string[],
  ) => Promise<InstalledDesktopApp | null>
  setAlwaysOnTop: (appId: string, enabled: boolean) => Promise<boolean>
  showNotification: (appId: string, title: string, body: string) => Promise<boolean>
  registerShortcut: (appId: string, accelerator: string) => Promise<boolean>
  setTray: (appId: string, enabled: boolean, title?: string) => Promise<boolean>
  pickScopedFolder: (appId: string) => Promise<string | null>
  fsList: (appId: string, folderPath: string, relPath?: string) => Promise<DesktopAppFSEntry[]>
  fsRead: (appId: string, folderPath: string, relPath: string) => Promise<string>
  fsWrite: (appId: string, folderPath: string, relPath: string, content: string) => Promise<boolean>
}

// Reaching the user's files or running programs, which apps from outside the org never get.
export function isMachineAccess(permission: DesktopAppPermission): boolean {
  return permission === 'fs' || permission === 'full_access'
}

// The desktop permissions an app declares as desktop:<permission> capabilities.
export function declaredDesktopPermissions(capabilities: readonly string[] = []): DesktopAppPermission[] {
  return DESKTOP_APP_PERMISSIONS.filter((permission) => capabilities.includes(`desktop:${permission}`))
}

export interface DesktopAppScope {
  workspaceId: string
  origin: string
  isLocalOrigin?: boolean
}

function normalizeScopeOrigin(raw?: string): string {
  const trimmed = (raw ?? '').trim().replace(/\/+$/, '')
  if (!trimmed) return ''
  try {
    return new URL(trimmed).origin.toLowerCase()
  } catch {
    return trimmed.toLowerCase()
  }
}

// Installed apps are stored in one desktop registry per machine; only records
// belonging to the active workspace and backend origin surface in its library.
export function matchesDesktopAppScope(
  record: Pick<InstalledDesktopApp, 'workspaceId' | 'origin'>,
  scope: DesktopAppScope,
): boolean {
  const recordWorkspace = (record.workspaceId || 'local').trim()
  const scopeWorkspace = (scope.workspaceId || 'local').trim()
  if (recordWorkspace !== scopeWorkspace) return false
  const recordOrigin = normalizeScopeOrigin(record.origin)
  const scopeOrigin = normalizeScopeOrigin(scope.origin)
  if (recordOrigin) {
    return Boolean(scopeOrigin) && recordOrigin === scopeOrigin
  }
  if (scopeWorkspace === 'local') {
    return scope.isLocalOrigin !== false
  }
  return true
}
