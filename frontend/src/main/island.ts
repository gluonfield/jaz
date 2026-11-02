import { join } from 'node:path'
import { app, BrowserWindow, ipcMain, powerMonitor, screen, type Display } from 'electron'
import {
  alertLabel,
  alertPath,
  alertSender,
  ISLAND_CAPSULE_WIDTH,
  ISLAND_CHANNEL,
  ISLAND_EAR_WIDTH,
  ISLAND_HEIGHT,
  ISLAND_HOLD_MS,
  type Alert,
  type IslandCommand,
  type IslandState,
} from '@shared/notifications'
import { islandShape, mergeRows } from './islandPolicy'
import { announce, notchStatus } from './notchHelper'

type Placement = { display: Display; notch: { x: number; width: number; height: number } | null; ears: boolean }

const ROWS = 3
const RETRACT_MS = 260

// The completion island beside the notch: one non-activating panel that
// merges arrivals, holds for ten seconds (paused while hovered) and keeps
// nothing once it retracts.
export function createIsland(open: (path: string) => void) {
  let window: BrowserWindow | null = null
  let placement: Placement | null = null
  let rows: Alert[] = []
  let deadline = 0
  let remaining = 0
  let timer: NodeJS.Timeout | null = null
  let hidden: NodeJS.Timeout | null = null
  let hovering = false

  const state = (): IslandState => ({
    rows: rows.slice(0, ROWS),
    more: Math.max(0, rows.length - ROWS),
    ears: placement?.ears ?? false,
    notch: placement?.notch ? { width: placement.notch.width, height: placement.notch.height } : null,
  })
  const publish = () => window?.webContents.send(`${ISLAND_CHANNEL}state`, state())

  function hold(ms: number): void {
    if (timer) clearTimeout(timer)
    remaining = ms
    deadline = Date.now() + ms
    timer = hovering ? null : setTimeout(retract, ms)
  }

  function retract(): void {
    if (timer) clearTimeout(timer)
    timer = null
    rows = []
    hovering = false
    window?.setIgnoreMouseEvents(true, { forward: true })
    publish()
    if (hidden) clearTimeout(hidden)
    hidden = setTimeout(() => {
      if (rows.length) return
      window?.hide()
      placement = null
    }, RETRACT_MS)
  }

  function remove(sessionId: string): void {
    rows = rows.filter((row) => row.sessionId !== sessionId)
    if (rows.length) publish()
    else retract()
  }

  function ensureWindow(): BrowserWindow {
    if (window && !window.isDestroyed()) return window
    const win = new BrowserWindow({
      title: 'Jaz',
      show: false,
      frame: false,
      transparent: true,
      resizable: false,
      movable: false,
      focusable: false,
      acceptFirstMouse: true,
      fullscreenable: false,
      minimizable: false,
      maximizable: false,
      skipTaskbar: true,
      hasShadow: false,
      enableLargerThanScreen: true,
      backgroundColor: '#00000000',
      type: 'panel',
      webPreferences: {
        preload: join(__dirname, '../preload/index.js'),
        contextIsolation: true,
        nodeIntegration: false,
        sandbox: false,
        backgroundThrottling: false,
        additionalArguments: ['--jaz-island-window'],
      },
    })
    win.setAlwaysOnTop(true, 'status')
    win.setVisibleOnAllWorkspaces(true, { visibleOnFullScreen: false, skipTransformProcessType: true })
    win.setContentProtection(true)
    win.setIgnoreMouseEvents(true, { forward: true })
    win.once('closed', () => {
      window = null
    })
    if (process.env.ELECTRON_RENDERER_URL) void win.loadURL(process.env.ELECTRON_RENDERER_URL)
    else void win.loadFile(join(__dirname, '../renderer/index.html'))
    window = win
    return win
  }

  // Where the island may appear now, or null when a standard notification
  // should go instead: without notification permission, on a locked or idle
  // Mac, with Focus on or unknown, in a full-screen space or on a Mac that may
  // have Apple's Dynamic Island.
  async function place(ears: boolean): Promise<Placement | null> {
    if (process.platform !== 'darwin' || powerMonitor.getSystemIdleState(60) !== 'active') return null
    const status = await notchStatus()
    if (status?.notifications !== 'authorized' || status.focus !== 'off') return null
    const display = screen.getDisplayNearestPoint(screen.getCursorScreenPoint())
    if (display.workArea.y === display.bounds.y) return null
    const notch = status.screens.find((item) => item.id === display.id)?.notch ?? null
    const shape = islandShape(status.model, display.internal, notch !== null)
    if (!shape) return null
    return { display, notch: shape === 'notch' ? notch : null, ears: ears || status.previews === 'never' }
  }

  function bounds({ display, notch }: Placement) {
    if (notch) {
      return { x: display.bounds.x + notch.x - ISLAND_EAR_WIDTH, y: display.bounds.y, width: notch.width + 2 * ISLAND_EAR_WIDTH, height: ISLAND_HEIGHT }
    }
    const { workArea } = display
    return { x: Math.round(workArea.x + (workArea.width - ISLAND_CAPSULE_WIDTH) / 2), y: workArea.y + 8, width: ISLAND_CAPSULE_WIDTH, height: ISLAND_HEIGHT }
  }

  ipcMain.on(`${ISLAND_CHANNEL}ready`, (event) => {
    if (event.sender === window?.webContents) publish()
  })
  ipcMain.on(`${ISLAND_CHANNEL}command`, (event, command: IslandCommand) => {
    if (event.sender !== window?.webContents || !command) return
    switch (command.type) {
      case 'hover':
        if (command.inside === hovering) return
        hovering = command.inside
        window.setIgnoreMouseEvents(!hovering, { forward: true })
        hold(hovering ? Math.max(0, deadline - Date.now()) : remaining)
        return
      case 'dismiss':
        remove(command.sessionId)
        return
      case 'open': {
        const row = rows.find((item) => item.sessionId === command.sessionId)
        if (!row) return
        remove(row.sessionId)
        open(alertPath(row))
        return
      }
      case 'more':
        retract()
        open('/feed')
    }
  })

  return {
    // Shows or merges an alert; false when it belongs in Notification Center.
    async present(alert: Alert, ears: boolean): Promise<boolean> {
      if (!rows.length) {
        const next = await place(ears)
        if (!next) return false
        placement = next
        const win = ensureWindow()
        win.setBounds(bounds(next))
        if (hidden) clearTimeout(hidden)
        win.showInactive()
      }
      rows = mergeRows(rows, alert)
      publish()
      hold(ISLAND_HOLD_MS)
      if (app.accessibilitySupportEnabled) {
        const label = `${alertSender(alert)}, ${alertLabel(alert).toLowerCase()}`
        announce(placement?.ears ? label : `${label}: ${alert.title || alert.body}`)
      }
      return true
    },
    clear(): void {
      if (rows.length) retract()
    },
  }
}
