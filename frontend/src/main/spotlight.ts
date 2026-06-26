import { execFile } from 'node:child_process'
import { readFile, rm } from 'node:fs/promises'
import { join } from 'node:path'
import { BrowserWindow, app, globalShortcut, ipcMain, screen } from 'electron'

// The ⌥Space launcher: a full-screen, transparent overlay over whatever app is
// frontmost. A composer bar floats near the top; dragging anywhere else selects
// a screen region that's captured natively and attached. Created lazily and
// reused — show/hide, never destroyed — so invocation stays instant.
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
    // A panel floats above fullscreen apps and takes key focus without fully
    // activating Jaz's other windows.
    type: 'panel',
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

// Cover the whole display under the cursor so a drag can select anywhere on it.
function coverCursorDisplay(win: BrowserWindow): void {
  win.setBounds(screen.getDisplayNearestPoint(screen.getCursorScreenPoint()).bounds)
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
  coverCursorDisplay(win)
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

// Capture a region the user dragged on the overlay. The rect is in the overlay
// window's coordinates; the window covers the display, so screen coords are just
// the window origin plus the rect. We hide the overlay first (so neither the bar
// nor the selection box land in the shot), let it clear, then grab the pixels.
async function captureScreenRect(rect: Rect): Promise<{ ok: boolean; data?: string }> {
  const win = launcher
  if (!win || win.isDestroyed()) return { ok: false }
  const w = Math.round(rect.width)
  const h = Math.round(rect.height)
  if (w < 2 || h < 2) return { ok: false }
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

// macOS-only for now: the region capture is native to macOS, and on Windows
// Alt+Space is the system window menu. A cross-platform path can register here.
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
