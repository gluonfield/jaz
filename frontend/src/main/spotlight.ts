import { execFile } from 'node:child_process'
import { readFile, rm } from 'node:fs/promises'
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
    skipTaskbar: true,
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
  win.setVisibleOnAllWorkspaces(true, { visibleOnFullScreen: true })
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
  coverCursorDisplay(win)
  presentLauncher(win)
  win.webContents.send('jaz:launcher-shown')
}

function hideLauncher(): void {
  if (!launcher || launcher.isDestroyed() || !launcher.isVisible()) return
  launcher.hide()
  // Return focus to the prior app, unless another Jaz window should keep it.
  if (!BrowserWindow.getAllWindows().some((win) => win !== launcher && win.isVisible())) {
    app.hide()
  }
}

function toggleLauncher(): void {
  if (launcher && !launcher.isDestroyed() && launcher.isVisible()) hideLauncher()
  else showLauncher()
}

let screenPromptShown = false

// Screen Recording permission only takes effect on relaunch; touching the
// capture API registers Jaz in the pane, then we open it.
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

async function captureScreenRect(rect: Rect): Promise<{ ok: boolean; data?: string; denied?: boolean }> {
  const win = launcher
  if (!win || win.isDestroyed()) return { ok: false }
  const w = Math.round(rect.width)
  const h = Math.round(rect.height)
  if (w < 2 || h < 2) return { ok: false }
  if (!(await ensureScreenPermission())) return { ok: false, denied: true }
  // The overlay covers the display, so screen coords are its origin plus the rect.
  const origin = win.getBounds()
  const x = Math.round(origin.x + rect.x)
  const y = Math.round(origin.y + rect.y)

  win.hide()
  await delay(120)
  const file = join(app.getPath('temp'), `jaz-shot-${process.pid}-${process.hrtime.bigint()}.png`)
  try {
    await new Promise<void>((resolve, reject) => {
      execFile('/usr/sbin/screencapture', ['-x', `-R${x},${y},${w},${h}`, '-t', 'png', file], (err) =>
        err ? reject(err) : resolve(),
      )
    })
    const data = await readFile(file)
      .then((bytes) => bytes.toString('base64'))
      .catch(() => '')
    return data ? { ok: true, data } : { ok: false }
  } catch {
    return { ok: false }
  } finally {
    await rm(file, { force: true }).catch(() => {})
    if (!win.isDestroyed()) presentLauncher(win)
  }
}

function isRect(value: unknown): value is Rect {
  const r = value as Rect
  return Boolean(r) && [r.x, r.y, r.width, r.height].every((n) => typeof n === 'number')
}

// macOS-only: native capture, and Alt+Space is the Windows system menu.
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
