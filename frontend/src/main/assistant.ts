import { spawn } from 'node:child_process'
import { join } from 'node:path'
import { BrowserWindow, ipcMain, screen, type Display, type IpcMainEvent } from 'electron'
import {
  ASSISTANT_CHANNEL,
  parseAssistantAction,
  type AssistantAction,
  type QuickDictateCommand,
  type QuickDictatePublishState,
  type QuickDictateState,
} from '@shared/assistant'
import { ISLAND_CAPSULE_WIDTH, ISLAND_EAR_WIDTH, ISLAND_HEIGHT, ISLAND_RETRACT_MS } from '@shared/notifications'
import { restoreWindowContentsFocus } from './diagnostics'
import { islandShape } from './islandPolicy'
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

function pillBounds({ display, notch }: Placement) {
  if (notch) {
    return {
      x: display.bounds.x + notch.x - ISLAND_EAR_WIDTH,
      y: display.bounds.y,
      width: notch.width + 2 * ISLAND_EAR_WIDTH,
      height: ISLAND_HEIGHT,
    }
  }
  const { workArea } = display
  return {
    x: Math.round(workArea.x + (workArea.width - ISLAND_CAPSULE_WIDTH) / 2),
    y: workArea.y + 8,
    width: ISLAND_CAPSULE_WIDTH,
    height: ISLAND_HEIGHT,
  }
}

export function createAssistant(getMainWindow: () => BrowserWindow | null, ensureMainWindow: () => BrowserWindow) {
  let owner: BrowserWindow | null = null
  let ownerReady = false
  let ownerThrottled = true
  let pending: AssistantAction | null = null
  let pill: BrowserWindow | null = null
  let pillLoaded = false
  let placement: Placement | null = null
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
      const serial = ++placementSerial
      void resolvePlacement().then((resolved) => {
        if (!current || serial !== placementSerial) return
        placement = resolved
        const win = ensurePill()
        win.setBounds(pillBounds(resolved))
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
    const win = getMainWindow() ?? ensureMainWindow()
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

  const onPublish = (event: IpcMainEvent, state: QuickDictatePublishState | null) => {
    if (!isOwner(event)) return
    updatePill(state)
  }

  const onPillReady = (event: IpcMainEvent) => {
    if (!isPill(event)) return
    pillLoaded = true
    publishToPill()
    if (current && placement && pill && !pill.isDestroyed() && !pill.isVisible()) {
      pill.showInactive()
    }
  }

  const onPillCommand = (event: IpcMainEvent, command: QuickDictateCommand) => {
    if (!isPill(event) || !command) return
    if (command.type === 'hover') {
      if (command.inside === hovering) return
      hovering = command.inside
      pill?.setIgnoreMouseEvents(!hovering, { forward: true })
      return
    }
    if (owner && !owner.isDestroyed()) {
      owner.webContents.send(`${ASSISTANT_CHANNEL}pill-command`, command)
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
    if (!owner || owner.isDestroyed()) {
      ensureMainWindow()
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
