import { readFileSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'
import {
  BrowserWindow,
  type Rectangle,
  app,
  ipcMain,
  nativeTheme,
  session,
  shell,
  systemPreferences,
} from 'electron'
import appIcon from '../assets/jaz-icon-1024.png?asset'
import { startLocalBackend, stopLocalBackend } from './backend'

// Matches --color-bg under :root.dark; used as the window paint color before
// the renderer mounts so a dark launch doesn't flash white behind the content.
const DARK_BG = '#1d1f24'

const APP_NAME = 'Jaz'

app.setName(APP_NAME)

let mainWindow: BrowserWindow | null = null

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
    },
  })

  mainWindow = win
  win.on('closed', () => {
    if (mainWindow === win) mainWindow = null
  })
  win.once('ready-to-show', () => win.show())

  // External links open in the system browser, never in the app.
  win.webContents.setWindowOpenHandler(({ url }) => {
    shell.openExternal(url)
    return { action: 'deny' }
  })

  if (process.env['ELECTRON_RENDERER_URL']) {
    win.loadURL(process.env['ELECTRON_RENDERER_URL'])
  } else {
    win.loadFile(join(__dirname, '../renderer/index.html'))
  }
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
