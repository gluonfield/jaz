import { join } from 'node:path'
import { BrowserWindow, ipcMain, screen, type IpcMainEvent } from 'electron'
import type { VoiceCommand, VoiceOverlayState } from '@shared/voice'

export function attachVoiceOverlay(owner: BrowserWindow): void {
  let overlay: BrowserWindow | null = null
  let state: VoiceOverlayState | null = null
  let loaded = false
  let dragOrigin: { x: number; y: number; windowX: number; windowY: number } | null = null
  const throttled = owner.webContents.getBackgroundThrottling()

  const present = () => {
    if (!state || (state.docked && owner.isFocused())) {
      overlay?.hide()
      return
    }
    if (!overlay) {
      const { workArea } = screen.getDisplayNearestPoint(screen.getCursorScreenPoint())
      overlay = new BrowserWindow({
        title: 'Jaz Voice',
        width: 220,
        height: 276,
        x: workArea.x + workArea.width - 240,
        y: workArea.y + workArea.height - 296,
        show: false,
        frame: false,
        transparent: true,
        resizable: false,
        fullscreenable: false,
        minimizable: false,
        maximizable: false,
        skipTaskbar: true,
        hasShadow: false,
        backgroundColor: '#00000000',
        ...(process.platform === 'darwin' ? { type: 'panel' as const } : {}),
        webPreferences: {
          preload: join(__dirname, '../preload/index.js'),
          contextIsolation: true,
          nodeIntegration: false,
          sandbox: false,
          backgroundThrottling: false,
          additionalArguments: ['--jaz-voice-window'],
        },
      })
      overlay.setAlwaysOnTop(true, 'floating')
      overlay.setVisibleOnAllWorkspaces(true, { visibleOnFullScreen: true, skipTransformProcessType: true })
      overlay.once('closed', () => {
        overlay = null
        loaded = false
        dragOrigin = null
      })
      if (process.env.ELECTRON_RENDERER_URL) {
        void overlay.loadURL(process.env.ELECTRON_RENDERER_URL)
      } else {
        void overlay.loadFile(join(__dirname, '../renderer/index.html'))
      }
    }
    if (loaded && !overlay.isVisible()) {
      overlay.showInactive()
    }
  }

  const update = (next: VoiceOverlayState | null) => {
    if (!next) {
      dragOrigin = null
    }
    if (Boolean(next) !== Boolean(state)) {
      owner.webContents.setBackgroundThrottling(next ? false : throttled)
    }
    state = next
    overlay?.webContents.send('jaz:voice:state', state)
    present()
  }
  const publish = (event: IpcMainEvent, next: VoiceOverlayState | null) => {
    if (event.sender === owner.webContents && event.senderFrame === owner.webContents.mainFrame) {
      update(next)
    }
  }
  const ready = (event: IpcMainEvent) => {
    if (event.sender !== overlay?.webContents || event.senderFrame !== overlay.webContents.mainFrame) {
      return
    }
    loaded = true
    event.sender.send('jaz:voice:state', state)
    present()
  }
  const command = (event: IpcMainEvent, action: VoiceCommand) => {
    if (!state || event.sender !== overlay?.webContents || event.senderFrame !== overlay.webContents.mainFrame) {
      return
    }
    if (action === 'return') {
      owner.show()
      owner.focus()
      owner.webContents.send('jaz:open-route', `/sessions/${encodeURIComponent(state.sessionId)}`)
    } else if (['mute', 'muteSpeaker', 'exit', 'reconnect'].includes(action)) {
      owner.webContents.send('jaz:voice:command', action)
    }
  }
  const clear = () => update(null)
  const drag = (event: IpcMainEvent, point: { x: number; y: number } | null) => {
    if (!state || !overlay || event.sender !== overlay.webContents || event.senderFrame !== overlay.webContents.mainFrame) {
      return
    }
    if (!point) {
      dragOrigin = null
      return
    }
    if (!Number.isFinite(point.x) || !Number.isFinite(point.y)) {
      return
    }
    if (!dragOrigin) {
      const [windowX, windowY] = overlay.getPosition()
      dragOrigin = { ...point, windowX, windowY }
    } else {
      overlay.setPosition(Math.round(dragOrigin.windowX + point.x - dragOrigin.x), Math.round(dragOrigin.windowY + point.y - dragOrigin.y))
    }
  }
  const navigate = (_event: Electron.Event, _url: string, sameDocument: boolean, mainFrame: boolean) => {
    if (mainFrame && !sameDocument) {
      clear()
    }
  }
  ipcMain.on('jaz:voice:publish', publish)
  ipcMain.on('jaz:voice:ready', ready)
  ipcMain.on('jaz:voice:command', command)
  ipcMain.on('jaz:voice:drag', drag)
  owner.on('focus', present)
  owner.on('blur', present)
  owner.on('hide', present)
  owner.webContents.on('did-start-navigation', navigate)
  owner.webContents.on('render-process-gone', clear)
  owner.once('closed', () => {
    ipcMain.removeListener('jaz:voice:publish', publish)
    ipcMain.removeListener('jaz:voice:ready', ready)
    ipcMain.removeListener('jaz:voice:command', command)
    ipcMain.removeListener('jaz:voice:drag', drag)
    overlay?.destroy()
  })
}
