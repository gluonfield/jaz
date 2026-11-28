import { spawn } from 'node:child_process'
import { join } from 'node:path'
import { app, BrowserWindow, ipcMain, screen, type Display, type IpcMainEvent } from 'electron'
import {
  ASSISTANT_CHANNEL,
  parseAssistantAction,
  type AssistantAction,
  type QuickDictateCommand,
  type QuickDictatePublishState,
  type QuickDictateState,
} from '@shared/assistant'
import { ISLAND_RETRACT_MS } from '@shared/notifications'
import { restoreWindowContentsFocus } from './diagnostics'
import { islandBounds, islandShape } from './islandPolicy'
import { notchStatus } from './notchHelper'

type Placement = {
  display: Display
  notch: { width: number; height: number; x: number } | null
}

const SOUND_START = '/System/Library/Sounds/Tink.aiff'
const SOUND_SENT = '/System/Library/Sounds/Pop.aiff'

function playCue(sound: string): void {
  if (process.platform !== 'darwin') return
  spawn('afplay', [sound], { stdio: 'ignore' }).on('error', () => {})
}

function isQuickDictateCommand(value: unknown): value is QuickDictateCommand {
  if (!value || typeof value !== 'object' || !('type' in value)) return false
  const cmd = value as { type: unknown; inside?: unknown }
  if (cmd.type === 'send' || cmd.type === 'cancel' || cmd.type === 'open') return true
  return cmd.type === 'hover' && typeof cmd.inside === 'boolean'
}

function isQuickDictatePublishState(value: unknown): value is QuickDictatePublishState {
  if (!value || typeof value !== 'object') return false
  const s = value as Record<string, unknown>
  return typeof s.phase === 'string' && typeof s.botName === 'string' && typeof s.text === 'string' && typeof s.level === 'number'
}

async function resolvePlacement(): Promise<Placement> {
  const display = screen.getDisplayNearestPoint(screen.getCursorScreenPoint())
  if (process.platform !== 'darwin') {
    return { display, notch: null }
  }
  const status = await notchStatus()
  const notch = status?.screens.find((item) => item.id === display.id)?.notch ?? null
  const shape = status ? islandShape(status.model, display.internal, notch !== null) : null
  return { display, notch: shape === 'notch' ? notch : null }
}

export function createAssistant(
  getMainWindow: () => BrowserWindow | null,
  ensureMainWindow: (showOnReady?: boolean) => BrowserWindow,
) {
  let owner: BrowserWindow | null = null
  let ownerReady = false
  let ownerThrottled = true
  let pending: AssistantAction | null = null
  let pill: BrowserWindow | null = null
  let pillLoaded = false
  let placement: Placement | null = null
  let placementReady = false
  let current: QuickDictatePublishState | null = null
  let hideTimer: NodeJS.Timeout | null = null
  let hovering = false
  let placementSerial = 0

  const isOwner = (event: IpcMainEvent) =>
    owner !== null && !owner.isDestroyed() && event.sender === owner.webContents && event.senderFrame === owner.webContents.mainFrame

  const isPill = (event: IpcMainEvent) =>
    pill !== null && !pill.isDestroyed() && event.sender === pill.webContents && event.senderFrame === pill.webContents.mainFrame

  const pillState = (): QuickDictateState | null =>
    current
      ? {
          ...current,
          notch: placement?.notch ? { width: placement.notch.width, height: placement.notch.height } : null,
        }
      : null

  const publishToPill = () => {
    if (!pill || pill.isDestroyed() || !pillLoaded) return
    if (current && !placementReady) return
    pill.webContents.send(`${ASSISTANT_CHANNEL}pill-state`, pillState())
  }

  function ensurePill(): BrowserWindow {
    if (pill && !pill.isDestroyed()) return pill
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
      ...(process.platform === 'darwin' ? { roundedCorners: false, type: 'panel' as const } : {}),
      enableLargerThanScreen: true,
      backgroundColor: '#00000000',
      webPreferences: {
        preload: join(__dirname, '../preload/index.js'),
        contextIsolation: true,
        nodeIntegration: false,
        sandbox: false,
        backgroundThrottling: false,
        additionalArguments: ['--jaz-quick-dictate-window'],
      },
    })
    win.setAlwaysOnTop(true, 'status')
    win.setVisibleOnAllWorkspaces(true, { visibleOnFullScreen: true, skipTransformProcessType: true })
    win.setIgnoreMouseEvents(true, { forward: true })
    win.once('closed', () => {
      pill = null
      pillLoaded = false
      hovering = false
    })
    if (process.env['ELECTRON_RENDERER_URL']) {
      void win.loadURL(process.env['ELECTRON_RENDERER_URL'])
    } else {
      void win.loadFile(join(__dirname, '../renderer/index.html'))
    }
    pill = win
    return win
  }

  function updatePill(next: QuickDictatePublishState | null): void {
    const prevPhase = current?.phase
    const wasActive = Boolean(current)
    const nowActive = Boolean(next)
    if (owner && !owner.isDestroyed() && wasActive !== nowActive) {
      owner.webContents.setBackgroundThrottling(nowActive ? false : ownerThrottled)
    }
    if (next?.phase === 'listening' && prevPhase !== 'listening') {
      playCue(SOUND_START)
    } else if (next?.phase === 'sent' && prevPhase !== 'sent') {
      playCue(SOUND_SENT)
    }
    current = next
    if (!next) {
      placementSerial += 1
      placementReady = false
      hovering = false
      pill?.setIgnoreMouseEvents(true, { forward: true })
      publishToPill()
      if (hideTimer) clearTimeout(hideTimer)
      hideTimer = setTimeout(() => {
        if (current) return
        pill?.hide()
        placement = null
        const focusedWin = BrowserWindow.getFocusedWindow?.()
        if (focusedWin && focusedWin !== pill) restoreWindowContentsFocus(focusedWin)
      }, ISLAND_RETRACT_MS)
      return
    }

    if (hideTimer) {
      clearTimeout(hideTimer)
      hideTimer = null
    }
    if (!wasActive) {
      placementReady = false
      const serial = ++placementSerial
      void resolvePlacement().then((resolved) => {
        if (!current || serial !== placementSerial) return
        placement = resolved
        placementReady = true
        const win = ensurePill()
        win.setBounds(islandBounds(resolved.display, resolved.notch))
        win.showInactive()
        publishToPill()
      })
      return
    }
    publishToPill()
  }

  function flushPending(): void {
    if (!pending || !owner || owner.isDestroyed() || !ownerReady) return
    const action = pending
    pending = null
    owner.webContents.send(`${ASSISTANT_CHANNEL}trigger`, action)
  }

  function focusMain(): void {
    const win = getMainWindow() ?? ensureMainWindow(true)
    if (win.isDestroyed()) return
    if (win.isMinimized()) win.restore()
    win.show()
    win.focus()
  }

  const onReady = (event: IpcMainEvent) => {
    if (!isOwner(event)) return
    ownerReady = true
    flushPending()
  }

  const onTrigger = (event: IpcMainEvent, raw: unknown) => {
    if (!isOwner(event)) return
    const action = parseAssistantAction(raw)
    if (action) dispatch(action)
  }

  const onFocusMain = (event: IpcMainEvent) => {
    if (!isOwner(event)) return
    focusMain()
  }

  const onPublish = (event: IpcMainEvent, raw: unknown) => {
    if (!isOwner(event)) return
    if (raw === null) updatePill(null)
    else if (isQuickDictatePublishState(raw)) updatePill(raw)
  }

  const onPillReady = (event: IpcMainEvent) => {
    if (!isPill(event)) return
    pillLoaded = true
    publishToPill()
    if (current && placementReady && pill && !pill.isDestroyed() && !pill.isVisible()) {
      pill.showInactive()
    }
  }

  const onPillCommand = (event: IpcMainEvent, raw: unknown) => {
    if (!isPill(event) || !isQuickDictateCommand(raw)) return
    if (raw.type === 'hover') {
      if (raw.inside === hovering) return
      hovering = raw.inside
      pill?.setIgnoreMouseEvents(!hovering, { forward: true })
      return
    }
    if (owner && !owner.isDestroyed()) {
      owner.webContents.send(`${ASSISTANT_CHANNEL}pill-command`, raw)
    }
  }

  ipcMain.on(`${ASSISTANT_CHANNEL}ready`, onReady)
  ipcMain.on(`${ASSISTANT_CHANNEL}trigger`, onTrigger)
  ipcMain.on(`${ASSISTANT_CHANNEL}focus-main`, onFocusMain)
  ipcMain.on(`${ASSISTANT_CHANNEL}publish`, onPublish)
  ipcMain.on(`${ASSISTANT_CHANNEL}pill-ready`, onPillReady)
  ipcMain.on(`${ASSISTANT_CHANNEL}pill-command`, onPillCommand)

  function dispatch(action: AssistantAction): void {
    pending = action
    if (!app.isReady()) return
    if (!owner || owner.isDestroyed()) {
      ensureMainWindow(action.mode === 'voice')
      return
    }
    flushPending()
  }

  function attach(win: BrowserWindow): void {
    owner = win
    ownerReady = false
    ownerThrottled = win.webContents.getBackgroundThrottling()
    win.webContents.on('did-start-navigation', (_event, _url, sameDocument, mainFrame) => {
      if (mainFrame && !sameDocument) {
        ownerReady = false
        updatePill(null)
      }
    })
    win.webContents.on('render-process-gone', () => {
      ownerReady = false
      updatePill(null)
    })
    win.once('closed', () => {
      if (owner === win) {
        owner = null
        ownerReady = false
        updatePill(null)
      }
    })
  }

  function teardown(): void {
    if (hideTimer) clearTimeout(hideTimer)
    ipcMain.removeListener(`${ASSISTANT_CHANNEL}ready`, onReady)
    ipcMain.removeListener(`${ASSISTANT_CHANNEL}trigger`, onTrigger)
    ipcMain.removeListener(`${ASSISTANT_CHANNEL}focus-main`, onFocusMain)
    ipcMain.removeListener(`${ASSISTANT_CHANNEL}publish`, onPublish)
    ipcMain.removeListener(`${ASSISTANT_CHANNEL}pill-ready`, onPillReady)
    ipcMain.removeListener(`${ASSISTANT_CHANNEL}pill-command`, onPillCommand)
    if (pill && !pill.isDestroyed()) pill.destroy()
    pill = null
  }

  return { attach, dispatch, teardown }
}
