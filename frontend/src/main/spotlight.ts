import { execFile } from 'node:child_process'
import { readFile, rm } from 'node:fs/promises'
import { join } from 'node:path'
import { BrowserWindow, app, globalShortcut, ipcMain, screen } from 'electron'

// A floating Spotlight-style launcher: ⌥Space pops a frameless, always-on-top
// card over any app, drafts a message (with optional screen-region screenshots),
// then hands the new session to the main window. The window is created lazily
// and reused — show/hide, never destroyed — so invocation stays instant.
const LAUNCHER_HASH = '/launcher'
const LAUNCHER_SHORTCUT = 'Alt+Space'
const WIDTH = 720
const HEIGHT = 260

let launcher: BrowserWindow | null = null

function preloadPath(): string {
  return join(__dirname, '../preload/index.js')
}

function buildLauncher(): BrowserWindow {
  const mac = process.platform === 'darwin'
  const win = new BrowserWindow({
    width: WIDTH,
    height: HEIGHT,
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
    // A panel floats above fullscreen apps and takes key focus without fully
    // activating Jaz's other windows.
    ...(mac ? { type: 'panel' as const } : {}),
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

// Center horizontally on the display under the cursor, anchored to its upper
// third — where a launcher reads as "in front of" rather than "on top of".
function positionLauncher(win: BrowserWindow): void {
  const { x, y, width, height } = screen.getDisplayNearestPoint(screen.getCursorScreenPoint()).workArea
  win.setPosition(Math.round(x + (width - WIDTH) / 2), Math.round(y + height * 0.22))
}

// Showing the window isn't enough when summoned over another app — Jaz must
// become the active app or keystrokes still go to whatever was frontmost.
function presentLauncher(win: BrowserWindow): void {
  win.show()
  app.focus({ steal: true })
  win.focus()
}

function showLauncher(): void {
  const win = ensureLauncher()
  positionLauncher(win)
  presentLauncher(win)
  win.webContents.send('jaz:launcher-shown')
}

function hideLauncher(): void {
  if (!launcher || launcher.isDestroyed() || !launcher.isVisible()) return
  launcher.hide()
  // Return focus to the app the user came from — but only when no other Jaz
  // window is up; otherwise let macOS pass focus to the remaining one (main).
  if (!BrowserWindow.getAllWindows().some((win) => win !== launcher && win.isVisible())) {
    app.hide()
  }
}

function toggleLauncher(): void {
  if (launcher && !launcher.isDestroyed() && launcher.isVisible()) hideLauncher()
  else showLauncher()
}

// macOS native region capture: `screencapture -i` gives the same crosshair the
// OS does (drag a rect or press Space to grab a window), writing a PNG only if
// the user commits. We hide the launcher first so it isn't in the shot, then
// restore it without re-emitting `launcher-shown` (which would clear the draft).
async function captureScreenRegion(): Promise<{ ok: boolean; data?: string }> {
  const restore = launcher?.isVisible() ?? false
  if (restore) launcher?.hide()
  const file = join(app.getPath('temp'), `jaz-shot-${process.pid}-${process.hrtime.bigint()}.png`)
  try {
    await new Promise<void>((resolve, reject) => {
      execFile('/usr/sbin/screencapture', ['-i', '-o', '-t', 'png', file], (err) =>
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
    if (restore && launcher && !launcher.isDestroyed()) presentLauncher(launcher)
  }
}

// macOS-only for now: the region capture is native to macOS, and on Windows
// Alt+Space is the system window menu. A cross-platform path can register here.
export function setupLauncher(): void {
  if (process.platform !== 'darwin') return
  ipcMain.handle('jaz:capture-screen-region', () => captureScreenRegion())
  ipcMain.on('jaz:hide-launcher', hideLauncher)
  globalShortcut.register(LAUNCHER_SHORTCUT, toggleLauncher)
}

export function teardownLauncher(): void {
  globalShortcut.unregisterAll()
}
