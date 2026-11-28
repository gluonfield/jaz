import { createHash } from 'node:crypto'
import { existsSync, readdirSync, readFileSync, statSync, writeFileSync } from 'node:fs'
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
  shell,
  type AppShimHost,
  type BrowserWindowConstructorOptions,
  type NativeImage,
  type Rectangle,
} from 'electron'
import {
  DESKTOP_APPS_CHANNEL,
  type DesktopAppFSEntry,
  type DesktopAppPermission,
  type InstallDesktopAppInput,
  type InstalledDesktopApp,
} from '@shared/desktopApps'
import { logMain } from './diagnostics'
import { trustedWindow } from './permissions'
import {
  VALID_APP_ID_RE,
  canUseDesktopPermission,
  filterPermissionsForPolicy,
  resolveScopedFolderTarget,
  syncDesktopAppVersionRecord,
} from './desktopAppsLogic'
import {
  appIconPath,
  appIdentity,
  createLauncher,
  removeLauncher,
  shimAppId,
  windowsArgs,
  type Launcher,
  type LaunchHost,
} from './desktopAppLaunchers'

const DARK_BG = '#141517'
const MAX_FS_READ_BYTES = 5 * 1024 * 1024

export interface DesktopAppsController {
  launch: (appId: string) => void
  teardown: () => void
}

export function installDesktopApps(deps: {
  preloadPath: string
  rendererPath: string
  appIcon: string
  attachWindow: (win: BrowserWindow, label: string) => void
}): DesktopAppsController {
  const userData = app.getPath('userData')
  const registryPath = join(userData, 'desktop-apps.json')
  const boundsPath = join(userData, 'desktop-app-windows.json')
  const host: LaunchHost = {
    executable: process.execPath,
    args: app.isPackaged ? [] : [app.getAppPath()],
    folder: `${app.getName()} Apps`,
    identity: app.isPackaged ? 'dev.wins.jaz.app' : 'dev.wins.jaz.dev.app',
    iconDir: join(userData, 'desktop-app-icons'),
    serverName: `jaz-apps-${createHash('sha256').update(userData).digest('hex').slice(0, 12)}`,
    writeShortcut: (path, details) => shell.writeShortcutLink(path, details),
  }
  const appWindows = new Map<string, BrowserWindow>()
  const appTrays = new Map<string, Tray>()
  const appShortcuts = new Map<string, string>()

  function loadRegistry(): Record<string, InstalledDesktopApp> {
    try {
      return JSON.parse(readFileSync(registryPath, 'utf8')) as Record<string, InstalledDesktopApp>
    } catch {
      return {}
    }
  }

  function saveRegistry(data: Record<string, InstalledDesktopApp>): void {
    writeFileSync(registryPath, JSON.stringify(data, null, 2), { encoding: 'utf8', mode: 0o600 })
  }

  function loadBounds(): Record<string, Rectangle> {
    try {
      return JSON.parse(readFileSync(boundsPath, 'utf8')) as Record<string, Rectangle>
    } catch {
      return {}
    }
  }

  function saveBounds(appId: string, bounds: Rectangle): void {
    try {
      writeFileSync(boundsPath, JSON.stringify({ ...loadBounds(), [appId]: bounds }))
    } catch {
      // ignore bounds persistence errors
    }
  }

  function writeLauncher(appId: string, title: string, icon: NativeImage): Launcher {
    const image = icon.isEmpty() ? nativeImage.createFromPath(deps.appIcon) : icon
    return createLauncher({ appId, title, png: (size) => image.resize({ width: size, height: size, quality: 'best' }).toPNG() }, host)
  }

  // Rebuilds a launcher that went missing, or, where launchers are app shims, one made before they were.
  function ensureLauncher(record: InstalledDesktopApp): string {
    if (record.launcherPath && existsSync(record.launcherPath) && (record.codeHash || !shimServer)) return record.launcherPath
    const icon = nativeImage.createFromPath(join(host.iconDir, `${record.appId}.png`))
    removeLauncher(host, record.appId, record.launcherPath)
    const registry = loadRegistry()
    const next = { ...record, ...writeLauncher(record.appId, record.title, icon) }
    registry[record.appId] = next
    saveRegistry(registry)
    return next.launcherPath
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

  // A window takes its launcher's identity: the macOS app shim it runs in, its own
  // Windows taskbar entry, or the Linux desktop entry it was created under.
  function createAppWindow(record: InstalledDesktopApp, options: BrowserWindowConstructorOptions, shim?: AppShimHost): BrowserWindow {
    const identity = appIdentity(host, record.appId)
    const desktopName = process.platform === 'linux' ? process.env['CHROME_DESKTOP'] : undefined
    if (desktopName !== undefined) app.setDesktopName(`${identity}.desktop`)
    try {
      const win = new BrowserWindow(shim ? { ...options, appShim: shim } : options)
      if (process.platform === 'win32') {
        win.setAppDetails({
          appId: identity,
          appIconPath: appIconPath(host, record.appId),
          relaunchCommand: `"${host.executable}" ${windowsArgs(host, record.appId)}`,
          relaunchDisplayName: record.title,
        })
      }
      return win
    } finally {
      if (desktopName !== undefined) app.setDesktopName(desktopName)
    }
  }

  function openAppWindow(appId: string, shim?: AppShimHost): void {
    const record = loadRegistry()[appId]
    if (!record) {
      shim?.quit()
      return
    }
    const existing = appWindows.get(appId)
    if (existing && !existing.isDestroyed()) {
      if (existing.isMinimized()) existing.restore()
      existing.show()
      existing.focus()
      return
    }
    const saved = loadBounds()[appId]
    const icon = appIconPath(host, appId)
    const win = createAppWindow(record, {
      title: record.title,
      width: saved?.width ?? 1024,
      height: saved?.height ?? 720,
      ...(saved ? { x: saved.x, y: saved.y } : {}),
      minWidth: 420,
      minHeight: 360,
      show: false,
      icon: existsSync(icon) ? icon : deps.appIcon,
      backgroundColor: nativeTheme.shouldUseDarkColors ? DARK_BG : '#ffffff',
      webPreferences: {
        preload: deps.preloadPath,
        contextIsolation: true,
        nodeIntegration: false,
        sandbox: false,
        additionalArguments: ['--jaz-app-window'],
      },
    }, shim)
    appWindows.set(appId, win)
    deps.attachWindow(win, `app ${appId}`)
    win.on('close', () => saveBounds(appId, win.getBounds()))
    win.on('closed', () => {
      appWindows.delete(appId)
      if (shim?.isConnected()) shim.quit()
    })
    if (process.env['ELECTRON_RENDERER_URL']) {
      void win.loadURL(`${process.env['ELECTRON_RENDERER_URL']}/apps/${appId}`)
    } else {
      void win.loadFile(deps.rendererPath, { hash: `/apps/${appId}` })
    }
  }

  // A macOS launcher is an app shim: its windows belong to its own Dock icon and menu bar.
  function startShimServer(): boolean {
    try {
      app.startAppShimServer({ name: host.serverName })
    } catch (error) {
      logMain('app shim server failed to start; apps open in Jaz windows', error)
      return false
    }
    const menu = Menu.buildFromTemplate([{ role: 'appMenu' }, { role: 'editMenu' }, { role: 'viewMenu' }, { role: 'windowMenu' }])
    app.on('app-shim-connected', (_event, shim) => {
      const appId = shimAppId(shim.bundlePath)
      const record = appId ? loadRegistry()[appId] : undefined
      if (!record?.codeHash || !shim.satisfiesCodeRequirement(`cdhash H"${record.codeHash}"`)) {
        logMain(`rejected app shim ${shim.bundlePath}`)
        shim.quit()
        return
      }
      shim.setApplicationMenu(menu)
      shim.on('activate', () => openAppWindow(record.appId, shim))
      shim.on('quit-requested', (event) => {
        const win = appWindows.get(record.appId)
        if (!win || win.isDestroyed()) return
        event.preventDefault()
        win.close()
      })
      openAppWindow(record.appId, shim)
    })
    // Jaz was updated while running, so its launchers can't connect until it restarts.
    app.on('app-shim-version-mismatch', (_event, bundlePath) => {
      const appId = shimAppId(bundlePath)
      if (appId) openAppWindow(appId)
    })
    return true
  }

  const shimServer = process.platform === 'darwin' && startShimServer()

  async function openApp(appId: string): Promise<void> {
    const record = loadRegistry()[appId]
    if (!record) throw new Error('This app is not installed')
    if (!shimServer) {
      openAppWindow(appId)
      return
    }
    const error = await shell.openPath(ensureLauncher(record))
    if (error) throw new Error(error)
  }

  const launch = (appId: string): void => {
    openApp(appId).catch((error: unknown) => logMain(`could not open app ${appId}`, error))
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
    const declared = input.declaredCapabilities ?? []
    const existing = registry[input.appId]
    removeLauncher(host, input.appId, existing?.launcherPath)
    const record: InstalledDesktopApp = {
      appId: input.appId,
      workspaceId: input.workspaceId || 'local',
      ...(input.origin?.trim() ? { origin: input.origin.trim() } : {}),
      title: input.title,
      description: input.description,
      kind: input.kind,
      icon: input.icon,
      installedVersion: input.version,
      approvedBundleSha256: input.bundleSha256,
      latestBundleSha256: input.bundleSha256,
      declaredCapabilities: declared,
      permissions: filterPermissionsForPolicy(input.permissions ?? [], declared, isExternal, Boolean(input.allowDesktopFullAccess)),
      scopedFolders: isExternal ? [] : (input.scopedFolders ?? []).filter((f) => typeof f === 'string' && f.trim()),
      permissionsPaused: false,
      isExternalOrg: isExternal,
      ...writeLauncher(input.appId, input.title, nativeImage.createFromDataURL(input.iconPng)),
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
      removeLauncher(host, appId, existing.launcherPath)
      delete registry[appId]
      saveRegistry(registry)
    }
    return { ok: true }
  })

  ipcMain.handle(`${DESKTOP_APPS_CHANNEL}open`, (event, appId: string) => {
    trustedWindow(event)
    return openApp(appId)
  })

  ipcMain.handle(`${DESKTOP_APPS_CHANNEL}reveal`, (event, appId: string) => {
    trustedWindow(event)
    const record = loadRegistry()[appId]
    if (!record) throw new Error('This app is not installed')
    shell.showItemInFolder(ensureLauncher(record))
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
      const filtered = filterPermissionsForPolicy(permissions, existing.declaredCapabilities, existing.isExternalOrg, allowDesktopFullAccess)
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
    notif.on('click', () => launch(appId))
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
    const ok = globalShortcut.register(accelerator.trim(), () => launch(appId))
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
          { label: `Open ${record.title}`, click: () => launch(appId) },
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
      tray.on('click', () => launch(appId))
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
    launch,
    teardown: () => {
      for (const appId of [...appShortcuts.keys()]) clearAppRuntimeGrants(appId)
      for (const appId of [...appTrays.keys()]) clearAppRuntimeGrants(appId)
    },
  }
}
