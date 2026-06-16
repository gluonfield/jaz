import { readFileSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'
import {
  BrowserWindow,
  Menu,
  type MenuItemConstructorOptions,
  type Rectangle,
  type WebContents,
  type WebPreferences,
  app,
  ipcMain,
  nativeTheme,
  session,
  shell,
  systemPreferences,
} from 'electron'
import { autoUpdater } from 'electron-updater'
import appIcon from '../assets/jaz-icon-1024.png?asset'
import { isPreviewURL } from '../shared/preview'
import type { UpdateStatus } from '../shared/update'
import { startLocalBackend, stopLocalBackend } from './backend'

// Matches --color-bg under :root.dark; used as the window paint color before
// the renderer mounts so a dark launch doesn't flash white behind the content.
const DARK_BG = '#1d1f24'

const APP_NAME = 'Jaz'
const UPDATE_CHECK_INTERVAL_MS = 6 * 60 * 60 * 1000

app.setName(APP_NAME)

let mainWindow: BrowserWindow | null = null
let updateTimer: NodeJS.Timeout | null = null
let updateStatus: UpdateStatus = { state: 'idle' }

function lockPreviewWebviewPreferences(webPreferences: WebPreferences): void {
  const prefs = webPreferences as WebPreferences & { preloadURL?: string }
  delete prefs.preload
  delete prefs.preloadURL
  webPreferences.nodeIntegration = false
  webPreferences.contextIsolation = true
  webPreferences.sandbox = true
  webPreferences.webSecurity = true
  webPreferences.allowRunningInsecureContent = false
}

function attachExternalOpenHandler(contents: WebContents): void {
  contents.setWindowOpenHandler(({ url }) => {
    if (contents.getType() === 'webview' && !isPreviewURL(url)) {
      return { action: 'deny' }
    }
    shell.openExternal(url)
    return { action: 'deny' }
  })
}

function attachPreviewNavigationGuard(contents: WebContents): void {
  contents.on('will-navigate', (event, url) => {
    if (contents.getType() !== 'webview' || isPreviewURL(url)) return
    event.preventDefault()
  })
}

// Electron ships no default right-click menu; build the standard link/text one.
function attachContextMenu(contents: WebContents): void {
  contents.on('context-menu', (_event, params) => {
    const editable = params.isEditable
    const selection = params.selectionText.trim()
    const linkURL = isPreviewURL(params.linkURL) ? params.linkURL : ''
    if (!linkURL && !editable && selection === '') return

    const items: MenuItemConstructorOptions[] = []
    if (linkURL) {
      items.push({
        label: 'Open in Browser',
        click: () => shell.openExternal(linkURL),
      })
      if (contents === mainWindow?.webContents) {
        items.push({
          label: 'Open in Side Browser',
          click: () => contents.send('jaz:open-side-browser-url', linkURL),
        })
      }
      if (editable || selection !== '') items.push({ type: 'separator' })
    }
    if (editable && params.misspelledWord) {
      for (const suggestion of params.dictionarySuggestions.slice(0, 5)) {
        items.push({ label: suggestion, click: () => contents.replaceMisspelling(suggestion) })
      }
      items.push(
        {
          label: 'Add to Dictionary',
          click: () => contents.session.addWordToSpellCheckerDictionary(params.misspelledWord),
        },
        { type: 'separator' },
      )
    }
    if (selection !== '') {
      const preview = selection.length > 30 ? `${selection.slice(0, 30)}…` : selection
      if (process.platform === 'darwin') {
        items.push({
          label: `Look Up “${preview}”`,
          click: () => contents.showDefinitionForSelection(),
        })
      }
      items.push(
        {
          label: `Search with Google “${preview}”`,
          click: () =>
            shell.openExternal(`https://www.google.com/search?q=${encodeURIComponent(selection)}`),
        },
        { type: 'separator' },
      )
    }
    if (editable) {
      items.push(
        { role: 'cut', enabled: params.editFlags.canCut },
        { role: 'copy', enabled: params.editFlags.canCopy },
        { role: 'paste', enabled: params.editFlags.canPaste },
        { role: 'selectAll', enabled: params.editFlags.canSelectAll },
      )
    } else {
      items.push({ role: 'copy', enabled: params.editFlags.canCopy })
    }
    Menu.buildFromTemplate(items).popup({
      window: BrowserWindow.fromWebContents(contents) ?? undefined,
    })
  })
}

// Covers every window — main, preview webviews, and per-board windows.
app.on('web-contents-created', (_event, contents) => {
  attachContextMenu(contents)
  attachExternalOpenHandler(contents)
  attachPreviewNavigationGuard(contents)
})

function createWindow(): void {
  const mac = process.platform === 'darwin'
  const win = new BrowserWindow({
    title: APP_NAME,
    width: 1280,
    height: 832,
    minWidth: 940,
    minHeight: 600,
    show: false,
    icon: appIcon,
    // On macOS the window paints the native sidebar material; the renderer
    // keeps the content column opaque and lets the sidebar show through.
    backgroundColor: mac ? '#00000000' : nativeTheme.shouldUseDarkColors ? DARK_BG : '#ffffff',
    ...(mac ? { vibrancy: 'sidebar' as const, visualEffectState: 'followWindow' as const } : {}),
    titleBarStyle: mac ? 'hiddenInset' : 'default',
    trafficLightPosition: { x: 18, y: 18 },
    webPreferences: {
      preload: join(__dirname, '../preload/index.js'),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: false,
      webviewTag: true,
    },
  })

  mainWindow = win
  win.on('closed', () => {
    if (mainWindow === win) mainWindow = null
  })
  win.once('ready-to-show', () => win.show())
  win.webContents.once('did-finish-load', () => {
    if (updateStatus.state !== 'idle') win.webContents.send('jaz:update-status', updateStatus)
  })

  win.webContents.on('will-attach-webview', (event, webPreferences, params) => {
    if (!isPreviewURL(params.src)) {
      event.preventDefault()
      return
    }
    lockPreviewWebviewPreferences(webPreferences)
  })

  if (process.env['ELECTRON_RENDERER_URL']) {
    win.loadURL(process.env['ELECTRON_RENDERER_URL'])
  } else {
    win.loadFile(join(__dirname, '../renderer/index.html'))
  }
}

function updateVersion(info: { version?: string }): string | undefined {
  return typeof info.version === 'string' && info.version !== '' ? info.version : undefined
}

function currentUpdateVersion(): string | undefined {
  return 'version' in updateStatus ? updateStatus.version : undefined
}

function setUpdateStatus(status: UpdateStatus): void {
  updateStatus = status
  mainWindow?.webContents.send('jaz:update-status', status)
}

function checkForUpdates(): void {
  void autoUpdater.checkForUpdates().catch((err: unknown) => {
    console.error('Jaz update check failed', err)
  })
}

function startAutoUpdater(): void {
  if (!app.isPackaged || updateTimer) return
  autoUpdater.autoDownload = true
  autoUpdater.autoInstallOnAppQuit = true
  autoUpdater.on('update-available', (info) =>
    setUpdateStatus({ state: 'available', version: updateVersion(info) }),
  )
  autoUpdater.on('download-progress', (info) => {
    const percent = Number.isFinite(info.percent) ? Math.max(0, Math.min(100, info.percent)) : 0
    setUpdateStatus({ state: 'downloading', percent, version: currentUpdateVersion() })
  })
  autoUpdater.on('update-downloaded', (info) =>
    setUpdateStatus({ state: 'downloaded', version: updateVersion(info) }),
  )
  autoUpdater.on('update-not-available', () => setUpdateStatus({ state: 'idle' }))
  autoUpdater.on('error', (err) => {
    console.error('Jaz updater error', err)
    if (updateStatus.state !== 'idle') {
      setUpdateStatus({
        state: 'error',
        message: err instanceof Error ? err.message : String(err),
      })
    }
  })
  setTimeout(checkForUpdates, 10_000).unref()
  updateTimer = setInterval(checkForUpdates, UPDATE_CHECK_INTERVAL_MS)
  updateTimer.unref()
}

// Deep link from a board window into the main app (e.g. "Open loop").
function openInMain(path: string): void {
  if (mainWindow && !mainWindow.isDestroyed()) {
    mainWindow.show()
    mainWindow.focus()
    mainWindow.webContents.send('jaz:open-route', path)
    return
  }
  createWindow()
  mainWindow?.webContents.once('did-finish-load', () => {
    mainWindow?.webContents.send('jaz:open-route', path)
  })
}

// Board windows: one OS window per board, bounds remembered locally (they are
// device-specific, unlike the board's tile layout which lives in the backend).
const boardWindows = new Map<string, BrowserWindow>()
const boardBoundsFile = (): string => join(app.getPath('userData'), 'board-windows.json')

function loadBoardBounds(): Record<string, Rectangle> {
  try {
    return JSON.parse(readFileSync(boardBoundsFile(), 'utf8')) as Record<string, Rectangle>
  } catch {
    return {}
  }
}

function saveBoardBounds(boardId: string, bounds: Rectangle): void {
  try {
    writeFileSync(boardBoundsFile(), JSON.stringify({ ...loadBoardBounds(), [boardId]: bounds }))
  } catch {
    // bounds are a convenience; never fail the close over them
  }
}

function openBoardWindow(boardId: string): void {
  const existing = boardWindows.get(boardId)
  if (existing && !existing.isDestroyed()) {
    existing.show()
    existing.focus()
    return
  }
  const saved = loadBoardBounds()[boardId]
  const win = new BrowserWindow({
    title: 'Board — Jaz',
    width: saved?.width ?? 960,
    height: saved?.height ?? 640,
    ...(saved ? { x: saved.x, y: saved.y } : {}),
    minWidth: 420,
    minHeight: 320,
    show: false,
    icon: appIcon,
    backgroundColor: nativeTheme.shouldUseDarkColors ? DARK_BG : '#ffffff',
    webPreferences: {
      preload: join(__dirname, '../preload/index.js'),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: false,
      additionalArguments: ['--jaz-board-window'],
    },
  })
  boardWindows.set(boardId, win)
  win.once('ready-to-show', () => win.show())
  win.on('close', () => saveBoardBounds(boardId, win.getBounds()))
  win.on('closed', () => boardWindows.delete(boardId))
  win.webContents.setWindowOpenHandler(({ url }) => {
    shell.openExternal(url)
    return { action: 'deny' }
  })
  if (process.env['ELECTRON_RENDERER_URL']) {
    win.loadURL(`${process.env['ELECTRON_RENDERER_URL']}/boards/${boardId}`)
  } else {
    win.loadFile(join(__dirname, '../renderer/index.html'), { hash: `/boards/${boardId}` })
  }
}

app.whenReady().then(() => {
  // Renderer mirrors its theme choice here so the native chrome (macOS traffic
  // lights, native scrollbars) and any new window's paint color match.
  ipcMain.on('jaz:set-native-theme', (_event, source) => {
    if (source === 'light' || source === 'dark' || source === 'system') {
      nativeTheme.themeSource = source
    }
  })

  ipcMain.handle('jaz:start-local-backend', () => startLocalBackend())
  ipcMain.handle('jaz:get-update-status', () => updateStatus)
  ipcMain.handle('jaz:install-update', () => {
    if (updateStatus.state !== 'downloaded') return { ok: false, error: 'update is not ready' }
    autoUpdater.quitAndInstall()
    return { ok: true }
  })

  ipcMain.on('jaz:open-board-window', (_event, boardId) => {
    if (typeof boardId === 'string' && boardId !== '') openBoardWindow(boardId)
  })

  ipcMain.on('jaz:open-in-main', (_event, path) => {
    if (typeof path === 'string' && path.startsWith('/')) openInMain(path)
  })

  // Voice mode records from the mic; allow media (macOS still shows its own
  // TCC prompt) and deny everything else.
  session.defaultSession.setPermissionRequestHandler((_wc, permission, callback) => {
    callback(permission === 'media')
  })
  if (process.platform === 'darwin') {
    app.dock?.setIcon(appIcon)
    app.setAboutPanelOptions({
      applicationName: APP_NAME,
      applicationVersion: app.getVersion(),
      iconPath: appIcon,
    })
    void systemPreferences.askForMediaAccess('microphone')
  }
  createWindow()
  startAutoUpdater()
  app.on('activate', () => {
    if (BrowserWindow.getAllWindows().length === 0) createWindow()
  })
})

app.on('window-all-closed', () => {
  if (process.platform !== 'darwin') app.quit()
})

app.on('before-quit', () => {
  stopLocalBackend()
})
