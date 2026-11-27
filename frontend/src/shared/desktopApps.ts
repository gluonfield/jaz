export type DesktopAppTier2Permission =
  | 'notifications'
  | 'always_on_top'
  | 'shortcuts'
  | 'tray'

export type DesktopAppTier3Permission = 'fs' | 'full_access'

export type DesktopAppPermission = DesktopAppTier2Permission | DesktopAppTier3Permission

export interface DesktopAppIconSpec {
  kind?: string
  value?: string
  color?: string
  background?: string
}

export interface InstalledDesktopApp {
  appId: string
  workspaceId: string
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
  shortcutAccelerator?: string
  installedAt: string
  updatedAt: string
}

export interface InstallDesktopAppInput {
  appId: string
  workspaceId: string
  title: string
  description?: string
  kind: 'artifact' | 'document'
  icon?: DesktopAppIconSpec
  version: number
  bundleSha256: string
  declaredCapabilities: string[]
  permissions?: DesktopAppPermission[]
  scopedFolders?: string[]
  isExternalOrg?: boolean
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
  openWindow: (appId: string, opts?: { title?: string }) => Promise<void>
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

export function isTier3Permission(p: DesktopAppPermission): p is DesktopAppTier3Permission {
  return p === 'fs' || p === 'full_access'
}

export function hasElevatedPermissions(permissions: readonly DesktopAppPermission[]): boolean {
  return permissions.length > 0
}
