import { join } from 'node:path'
import {
  BrowserWindow,
  app,
  desktopCapturer,
  globalShortcut,
  ipcMain,
  screen,
  shell,
  systemPreferences,
} from 'electron'

const LAUNCHER_HASH = '/launcher'
const LAUNCHER_SHORTCUT = 'Alt+Space'

interface Rect {
  x: number
  y: number
  width: number
  height: number
}

let launcher: BrowserWindow | null = null
// Whether opening the launcher stole focus from another app, so dismissing it
// should return there instead of surfacing Jaz's own main window.
let stoleForeignFocus = false

function preloadPath(): string {
  return join(__dirname, '../preload/index.js')
}

function delay(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms))
}

function buildLauncher(): BrowserWindow {
  const { width, height } = screen.getPrimaryDisplay().bounds
  const win = new BrowserWindow({
    width,
    height,
    show: false,
    frame: false,
    transparent: true,
    resizable: false,
    fullscreenable: false,
    minimizable: false,
    maximizable: false,
    hasShadow: false,
    backgroundColor: '#00000000',
    // Not a 'panel': a regular window becomes key so the composer can type.
    webPreferences: {
      preload: preloadPath(),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: false,
      additionalArguments: ['--jaz-launcher-window'],
    },
  })
  win.setAlwaysOnTop(true, 'screen-saver')
  win.setVisibleOnAllWorkspaces(true, {
    visibleOnFullScreen: true,
    skipTransformProcessType: true,
  })
  win.on('blur', hideLauncher)
  win.on('closed', () => {
    if (launcher === win) launcher = null
  })
  if (process.env['ELECTRON_RENDERER_URL']) {
    void win.loadURL(`${process.env['ELECTRON_RENDERER_URL']}${LAUNCHER_HASH}`)
  } else {
    void win.loadFile(join(__dirname, '../renderer/index.html'), { hash: LAUNCHER_HASH })
  }
  return win
}

function ensureLauncher(): BrowserWindow {
  if (!launcher || launcher.isDestroyed()) launcher = buildLauncher()
  return launcher
}

function coverCursorDisplay(win: BrowserWindow): void {
  win.setBounds(screen.getDisplayNearestPoint(screen.getCursorScreenPoint()).bounds)
}

function presentLauncher(win: BrowserWindow): void {
  win.show()
  // Without stealing app focus, keystrokes go to the previously-frontmost app.
  app.focus({ steal: true })
  win.focus()
}

function showLauncher(): void {
  const win = ensureLauncher()
  stoleForeignFocus = BrowserWindow.getFocusedWindow() === null
  coverCursorDisplay(win)
  presentLauncher(win)
  win.webContents.send('jaz:launcher-shown')
}

function hideLauncher(): void {
  if (!launcher || launcher.isDestroyed() || !launcher.isVisible()) return
  launcher.hide()
  // Deactivate Jaz so focus returns to the app we opened over, rather than
  // letting macOS surface the main window as the app's next key window.
  if (stoleForeignFocus) app.hide()
}

function toggleLauncher(): void {
  if (launcher && !launcher.isDestroyed() && launcher.isVisible()) hideLauncher()
  else showLauncher()
}

let screenPromptShown = false

// Screen Recording permission only applies after relaunch.
async function ensureScreenPermission(): Promise<boolean> {
  if (systemPreferences.getMediaAccessStatus('screen') === 'granted') return true
  if (!screenPromptShown) {
    screenPromptShown = true
    await desktopCapturer
      .getSources({ types: ['screen'], thumbnailSize: { width: 1, height: 1 } })
      .catch(() => {})
    void shell.openExternal('x-apple.systempreferences:com.apple.preference.security?Privacy_ScreenCapture')
  }
  return systemPreferences.getMediaAccessStatus('screen') === 'granted'
}

// In-process capture uses Jaz's own grant; a `screencapture` subprocess does not.
async function captureScreenRect(rect: Rect): Promise<{ ok: boolean; data?: string; denied?: boolean }> {
  const win = launcher
  if (!win || win.isDestroyed()) return { ok: false }
  if (Math.round(rect.width) < 2 || Math.round(rect.height) < 2) return { ok: false }

  try {
    if (!(await ensureScreenPermission())) return { ok: false, denied: true }
    const display = screen.getDisplayMatching(win.getBounds())

    win.hide()
    await delay(120)
    const sources = await desktopCapturer.getSources({
      types: ['screen'],
      thumbnailSize: {
        width: Math.round(display.size.width * display.scaleFactor),
        height: Math.round(display.size.height * display.scaleFactor),
      },
    })
    const full = (sources.find((s) => String(s.display_id) === String(display.id)) ?? sources[0])?.thumbnail
    if (!full || full.isEmpty()) return { ok: false }
    const size = full.getSize()
    const sx = size.width / display.size.width
    const sy = size.height / display.size.height
    const x = clamp(Math.round(rect.x * sx), 0, size.width - 1)
    const y = clamp(Math.round(rect.y * sy), 0, size.height - 1)
    const shot = full.crop({
      x,
      y,
      width: clamp(Math.round(rect.width * sx), 1, size.width - x),
      height: clamp(Math.round(rect.height * sy), 1, size.height - y),
    })
    const data = shot.isEmpty() ? '' : shot.toPNG().toString('base64')
    return data ? { ok: true, data } : { ok: false }
  } catch {
    return { ok: false }
  } finally {
    if (!win.isDestroyed()) presentLauncher(win)
  }
}

function clamp(value: number, min: number, max: number): number {
  return Math.max(min, Math.min(value, max))
}

function isRect(value: unknown): value is Rect {
  const r = value as Rect
  return Boolean(r) && [r.x, r.y, r.width, r.height].every((n) => typeof n === 'number')
}

export function setupLauncher(): void {
  if (process.platform !== 'darwin') return
  ipcMain.handle('jaz:capture-screen-rect', (_event, rect) =>
    isRect(rect) ? captureScreenRect(rect) : { ok: false },
  )
  ipcMain.on('jaz:hide-launcher', hideLauncher)
  globalShortcut.register(LAUNCHER_SHORTCUT, toggleLauncher)
}

export function teardownLauncher(): void {
  globalShortcut.unregisterAll()
}
