import {
  readdirSync,
  readFileSync,
  statSync,
  writeFileSync,
} from 'node:fs'
import { join } from 'node:path'
import {
  BrowserWindow,
  Menu,
  Notification,
  Tray,
  app,
  dialog,
  globalShortcut,
  ipcMain,
  nativeImage,
  nativeTheme,
  type Rectangle,
} from 'electron'
import {
  DESKTOP_APPS_CHANNEL,
  type DesktopAppFSEntry,
  type DesktopAppPermission,
  type InstallDesktopAppInput,
  type InstalledDesktopApp,
} from '@shared/desktopApps'
import { trustedWindow } from './permissions'
import {
  VALID_APP_ID_RE,
  canUseDesktopPermission,
  createOSLauncherShim,
  extractDesktopAppDeepLinkArg,
  filterPermissionsForPolicy,
  parseDesktopAppDeepLink,
  removeOSLauncherShim,
  resolveScopedFolderTarget,
  syncDesktopAppVersionRecord,
} from './desktopAppsLogic'

export {
  canUseDesktopPermission,
  createOSLauncherShim,
  extractDesktopAppDeepLinkArg,
  filterPermissionsForPolicy,
  parseDesktopAppDeepLink,
  removeOSLauncherShim,
  resolveScopedFolderTarget,
  syncDesktopAppVersionRecord,
}

const DARK_BG = '#141517'
const MAX_FS_READ_BYTES = 5 * 1024 * 1024

export interface DesktopAppsController {
  openAppWindow: (appId: string, opts?: { title?: string }) => void
  teardown: () => void
}

export function installDesktopApps(deps: {
  preloadPath: string
  rendererPath: string
  appIcon: string
  attachWindow: (win: BrowserWindow, label: string) => void
}): DesktopAppsController {
  const registryPath = (): string => join(app.getPath('userData'), 'desktop-apps.json')
  const boundsPath = (): string => join(app.getPath('userData'), 'desktop-app-windows.json')
  const appWindows = new Map<string, BrowserWindow>()
  const appTrays = new Map<string, Tray>()
  const appShortcuts = new Map<string, string>()

  function loadRegistry(): Record<string, InstalledDesktopApp> {
    try {
      return JSON.parse(readFileSync(registryPath(), 'utf8')) as Record<string, InstalledDesktopApp>
    } catch {
      return {}
    }
  }

  function saveRegistry(data: Record<string, InstalledDesktopApp>): void {
    writeFileSync(registryPath(), JSON.stringify(data, null, 2), 'utf8')
  }

  function loadBounds(): Record<string, Rectangle> {
    try {
      return JSON.parse(readFileSync(boundsPath(), 'utf8')) as Record<string, Rectangle>
    } catch {
      return {}
    }
  }

  function saveBounds(appId: string, bounds: Rectangle): void {
    try {
      writeFileSync(boundsPath(), JSON.stringify({ ...loadBounds(), [appId]: bounds }))
    } catch {
      // ignore bounds persistence errors
    }
  }

  function clearAppRuntimeGrants(appId: string): void {
    const accel = appShortcuts.get(appId)
    if (accel) {
      try {
        globalShortcut.unregister(accel)
      } catch {
        // ignore
      }
      appShortcuts.delete(appId)
    }
    const tray = appTrays.get(appId)
    if (tray) {
      tray.destroy()
      appTrays.delete(appId)
    }
    const win = appWindows.get(appId)
    if (win && !win.isDestroyed()) {
      win.setAlwaysOnTop(false)
    }
  }

  function openAppWindow(appId: string, opts?: { title?: string }): void {
    if (!VALID_APP_ID_RE.test(appId)) return
    const existing = appWindows.get(appId)
    if (existing && !existing.isDestroyed()) {
      if (existing.isMinimized()) existing.restore()
      existing.show()
      existing.focus()
      return
    }
    const record = loadRegistry()[appId]
    const saved = loadBounds()[appId]
    const win = new BrowserWindow({
      title: opts?.title || record?.title || 'Jaz App',
      width: saved?.width ?? 1024,
      height: saved?.height ?? 720,
      ...(saved ? { x: saved.x, y: saved.y } : {}),
      minWidth: 420,
      minHeight: 360,
      show: false,
      icon: deps.appIcon,
      backgroundColor: nativeTheme.shouldUseDarkColors ? DARK_BG : '#ffffff',
      webPreferences: {
        preload: deps.preloadPath,
        contextIsolation: true,
        nodeIntegration: false,
        sandbox: false,
        additionalArguments: ['--jaz-app-window'],
      },
    })
    appWindows.set(appId, win)
    deps.attachWindow(win, `app ${appId}`)
    win.on('close', () => saveBounds(appId, win.getBounds()))
    win.on('closed', () => appWindows.delete(appId))
    if (process.env['ELECTRON_RENDERER_URL']) {
      void win.loadURL(`${process.env['ELECTRON_RENDERER_URL']}/apps/${appId}`)
    } else {
      void win.loadFile(deps.rendererPath, { hash: `/apps/${appId}` })
    }
  }

  ipcMain.handle(`${DESKTOP_APPS_CHANNEL}list`, (event) => {
    trustedWindow(event)
    return Object.values(loadRegistry()).sort((a, b) => b.updatedAt.localeCompare(a.updatedAt))
  })

  ipcMain.handle(`${DESKTOP_APPS_CHANNEL}get`, (event, appId: string) => {
    trustedWindow(event)
    return loadRegistry()[appId] ?? null
  })

  ipcMain.handle(`${DESKTOP_APPS_CHANNEL}install`, (event, input: InstallDesktopAppInput) => {
    trustedWindow(event)
    if (!VALID_APP_ID_RE.test(input.appId)) {
      throw new Error('Invalid appId')
    }
    const registry = loadRegistry()
    const now = new Date().toISOString()
    const isExternal = Boolean(input.isExternalOrg)
    const perms = filterPermissionsForPolicy(
      input.permissions ?? [],
      isExternal,
      Boolean(input.allowDesktopFullAccess),
    )
    const folders = isExternal ? [] : (input.scopedFolders ?? []).filter((f) => typeof f === 'string' && f.trim())
    const existing = registry[input.appId]
    if (existing?.launcherPath) {
      removeOSLauncherShim(existing.launcherPath)
    }
    const launcherPath = createOSLauncherShim(input.title, input.appId)
    const record: InstalledDesktopApp = {
      appId: input.appId,
      workspaceId: input.workspaceId || 'local',
      title: input.title,
      description: input.description,
      kind: input.kind,
      icon: input.icon,
      installedVersion: input.version,
      approvedBundleSha256: input.bundleSha256,
      latestBundleSha256: input.bundleSha256,
      declaredCapabilities: input.declaredCapabilities ?? [],
      permissions: perms,
      scopedFolders: folders,
      permissionsPaused: false,
      isExternalOrg: isExternal,
      launcherPath,
      installedAt: existing?.installedAt ?? now,
      updatedAt: now,
    }
    registry[input.appId] = record
    saveRegistry(registry)
    return record
  })

  ipcMain.handle(`${DESKTOP_APPS_CHANNEL}uninstall`, (event, appId: string) => {
    trustedWindow(event)
    const registry = loadRegistry()
    const existing = registry[appId]
    clearAppRuntimeGrants(appId)
    const win = appWindows.get(appId)
    if (win && !win.isDestroyed()) {
      win.close()
    }
    if (existing) {
      removeOSLauncherShim(existing.launcherPath)
      delete registry[appId]
      saveRegistry(registry)
    }
    return { ok: true }
  })

  ipcMain.handle(`${DESKTOP_APPS_CHANNEL}openWindow`, (event, appId: string, opts?: { title?: string }) => {
    trustedWindow(event)
    openAppWindow(appId, opts)
  })

  ipcMain.handle(
    `${DESKTOP_APPS_CHANNEL}setPermissions`,
    (
      event,
      appId: string,
      bundleSha256: string,
      permissions: DesktopAppPermission[],
      scopedFolders?: string[],
      allowDesktopFullAccess = false,
    ) => {
      trustedWindow(event)
      const registry = loadRegistry()
      const existing = registry[appId]
      if (!existing) throw new Error('App is not installed')
      const filtered = filterPermissionsForPolicy(permissions, existing.isExternalOrg, allowDesktopFullAccess)
      const next: InstalledDesktopApp = {
        ...existing,
        approvedBundleSha256: bundleSha256 || existing.latestBundleSha256,
        latestBundleSha256: bundleSha256 || existing.latestBundleSha256,
        permissions: filtered,
        scopedFolders: existing.isExternalOrg ? [] : (scopedFolders ?? existing.scopedFolders),
        permissionsPaused: false,
        updatedAt: new Date().toISOString(),
      }
      if (!filtered.includes('always_on_top')) {
        const win = appWindows.get(appId)
        if (win && !win.isDestroyed()) win.setAlwaysOnTop(false)
      }
      if (!filtered.includes('shortcuts')) {
        const accel = appShortcuts.get(appId)
        if (accel) {
          globalShortcut.unregister(accel)
          appShortcuts.delete(appId)
        }
      }
      if (!filtered.includes('tray')) {
        const tray = appTrays.get(appId)
        if (tray) {
          tray.destroy()
          appTrays.delete(appId)
        }
      }
      registry[appId] = next
      saveRegistry(registry)
      return next
    },
  )

  ipcMain.handle(
    `${DESKTOP_APPS_CHANNEL}syncVersion`,
    (event, appId: string, version: number, bundleSha256: string, declaredCapabilities?: string[]) => {
      trustedWindow(event)
      const registry = loadRegistry()
      const existing = registry[appId]
      if (!existing) return null
      const synced = syncDesktopAppVersionRecord(existing, version, bundleSha256, declaredCapabilities)
      if (synced.permissionsPaused) {
        clearAppRuntimeGrants(appId)
      }
      registry[appId] = synced
      saveRegistry(registry)
      return synced
    },
  )

  ipcMain.handle(`${DESKTOP_APPS_CHANNEL}setAlwaysOnTop`, (event, appId: string, enabled: boolean) => {
    trustedWindow(event)
    const record = loadRegistry()[appId]
    if (!canUseDesktopPermission(record, 'always_on_top')) return false
    const win = appWindows.get(appId) ?? BrowserWindow.fromWebContents(event.sender)
    if (!win || win.isDestroyed()) return false
    win.setAlwaysOnTop(Boolean(enabled))
    return true
  })

  ipcMain.handle(`${DESKTOP_APPS_CHANNEL}showNotification`, (event, appId: string, title: string, body: string) => {
    trustedWindow(event)
    const record = loadRegistry()[appId]
    if (!canUseDesktopPermission(record, 'notifications')) return false
    if (!Notification.isSupported()) return false
    const notif = new Notification({ title: title || record.title, body: body || '' })
    notif.on('click', () => openAppWindow(appId, { title: record.title }))
    notif.show()
    return true
  })

  ipcMain.handle(`${DESKTOP_APPS_CHANNEL}registerShortcut`, (event, appId: string, accelerator: string) => {
    trustedWindow(event)
    const registry = loadRegistry()
    const record = registry[appId]
    if (!canUseDesktopPermission(record, 'shortcuts')) return false
    const prev = appShortcuts.get(appId)
    if (prev) globalShortcut.unregister(prev)
    if (!accelerator.trim()) {
      appShortcuts.delete(appId)
      return true
    }
    const ok = globalShortcut.register(accelerator.trim(), () => openAppWindow(appId, { title: record.title }))
    if (ok) {
      appShortcuts.set(appId, accelerator.trim())
      registry[appId] = { ...record, shortcutAccelerator: accelerator.trim(), updatedAt: new Date().toISOString() }
      saveRegistry(registry)
    }
    return ok
  })

  ipcMain.handle(`${DESKTOP_APPS_CHANNEL}setTray`, (event, appId: string, enabled: boolean, title?: string) => {
    trustedWindow(event)
    const record = loadRegistry()[appId]
    if (!canUseDesktopPermission(record, 'tray')) return false
    const existing = appTrays.get(appId)
    if (!enabled) {
      if (existing) {
        existing.destroy()
        appTrays.delete(appId)
      }
      return true
    }
    if (!existing) {
      const icon = nativeImage.createFromPath(deps.appIcon).resize({ width: 16, height: 16 })
      const tray = new Tray(icon)
      tray.setToolTip(title || record.title)
      tray.setContextMenu(
        Menu.buildFromTemplate([
          { label: `Open ${record.title}`, click: () => openAppWindow(appId, { title: record.title }) },
          { type: 'separator' },
          {
            label: 'Remove from menu bar',
            click: () => {
              tray.destroy()
              appTrays.delete(appId)
            },
          },
        ]),
      )
      tray.on('click', () => openAppWindow(appId, { title: record.title }))
      appTrays.set(appId, tray)
    }
    return true
  })

  ipcMain.handle(`${DESKTOP_APPS_CHANNEL}pickScopedFolder`, async (event, appId: string) => {
    trustedWindow(event)
    const record = loadRegistry()[appId]
    if (record?.isExternalOrg) return null
    const win = BrowserWindow.fromWebContents(event.sender)
    const res = win
      ? await dialog.showOpenDialog(win, { properties: ['openDirectory'] })
      : await dialog.showOpenDialog({ properties: ['openDirectory'] })
    return res.canceled ? null : (res.filePaths[0] ?? null)
  })

  ipcMain.handle(`${DESKTOP_APPS_CHANNEL}fsList`, (event, appId: string, folderPath: string, relPath = ''): DesktopAppFSEntry[] => {
    trustedWindow(event)
    const record = loadRegistry()[appId]
    const targetDir = resolveScopedFolderTarget(record, folderPath, relPath)
    return readdirSync(targetDir, { withFileTypes: true }).map((entry) => {
      let size = 0
      if (entry.isFile()) {
        try {
          size = statSync(join(targetDir, entry.name)).size
        } catch {
          size = 0
        }
      }
      return { name: entry.name, isDir: entry.isDirectory(), size }
    })
  })

  ipcMain.handle(`${DESKTOP_APPS_CHANNEL}fsRead`, (event, appId: string, folderPath: string, relPath: string): string => {
    trustedWindow(event)
    const record = loadRegistry()[appId]
    const targetFile = resolveScopedFolderTarget(record, folderPath, relPath)
    const st = statSync(targetFile)
    if (st.size > MAX_FS_READ_BYTES) {
      throw new Error('File exceeds 5 MiB desktop app read limit')
    }
    return readFileSync(targetFile, 'utf8')
  })

  ipcMain.handle(`${DESKTOP_APPS_CHANNEL}fsWrite`, (event, appId: string, folderPath: string, relPath: string, content: string): boolean => {
    trustedWindow(event)
    if (Buffer.byteLength(content, 'utf8') > MAX_FS_READ_BYTES) {
      throw new Error('File exceeds 5 MiB desktop app write limit')
    }
    const record = loadRegistry()[appId]
    const targetFile = resolveScopedFolderTarget(record, folderPath, relPath)
    writeFileSync(targetFile, content, 'utf8')
    return true
  })

  return {
    openAppWindow,
    teardown: () => {
      for (const appId of [...appShortcuts.keys()]) clearAppRuntimeGrants(appId)
      for (const appId of [...appTrays.keys()]) clearAppRuntimeGrants(appId)
    },
  }
}
