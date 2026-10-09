import { spawn } from 'node:child_process'
import { join, resolve } from 'node:path'
import { createInterface } from 'node:readline'
import { BrowserWindow, ipcMain, nativeImage, screen, shell, type IpcMainEvent } from 'electron'
import appIcon from '../assets/jaz-icon-1024.png?asset'
import { PERMISSION_GUIDE_CHANNEL, type ComputerPermission } from '@shared/computerControl'

const PANES: Record<ComputerPermission, string> = {
  accessibility: 'Privacy_Accessibility',
  screenRecording: 'Privacy_ScreenCapture',
}
// Where the privacy list sits inside the System Settings window on macOS 26.
const LIST = { left: 243, right: 20, firstRow: 108 }
// The panel docks beside the window with its row level with the list's first
// row (43px down the panel) and its arrow touching the window's right edge.
// Without room there it docks under the window, spanning the list's width
// inside a 12px margin, its arrow touching the bottom edge.
const SIDE = { width: 360, height: 150, row: 43 }
const BELOW = { margin: 12, height: 172 }

// Reports the largest on-screen System Settings window as "x y width height
// front" on every change, and "none" once a reported window has been gone for
// about half a second. The window server answers without any privacy
// permission, and the loop exits when its parent dies.
const TRACKER = `
ObjC.import('AppKit')
ObjC.import('CoreGraphics')
ObjC.import('unistd')
const parent = $.getppid()
const out = $.NSFileHandle.fileHandleWithStandardOutput
const layer0 = $.NSPredicate.predicateWithFormat('kCGWindowLayer == 0 AND kCGWindowAlpha > 0')
let last = 'none'
let misses = 0
while ($.getppid() === parent) {
  const apps = $.NSRunningApplication.runningApplicationsWithBundleIdentifier('com.apple.systempreferences')
  const windows = ObjC.castRefToObject($.CGWindowListCopyWindowInfo($.kCGWindowListOptionOnScreenOnly | $.kCGWindowListExcludeDesktopElements, 0)).filteredArrayUsingPredicate(layer0)
  let line = 'none'
  if (apps.count && windows.count) {
    const pid = apps.firstObject.processIdentifier
    const own = ObjC.deepUnwrap(windows.filteredArrayUsingPredicate($.NSPredicate.predicateWithFormat('kCGWindowOwnerPID == ' + pid)))
    const bounds = own.map((window) => window.kCGWindowBounds).sort((a, b) => b.Width * b.Height - a.Width * a.Height)[0]
    if (bounds) {
      const front = ObjC.unwrap(windows.firstObject.objectForKey('kCGWindowOwnerPID')) === pid
      line = [bounds.X, bounds.Y, bounds.Width, bounds.Height, front ? 1 : 0].join(' ')
    }
  }
  misses = line === 'none' ? misses + 1 : 0
  if (line !== last && (line !== 'none' || misses > 15)) {
    last = line
    out.writeData($(line + '\\n').dataUsingEncoding($.NSUTF8StringEncoding))
  }
  delay(1 / 30)
}
`

let close: (() => void) | undefined

// Opens the permission's System Settings list with a panel docked to the
// window: dragging Jaz from the panel into the list grants the permission.
export async function guidePermission(permission: ComputerPermission, granted: () => boolean): Promise<void> {
  close?.()
  const panel = new BrowserWindow({
    show: false,
    frame: false,
    transparent: true,
    resizable: false,
    movable: false,
    fullscreenable: false,
    minimizable: false,
    maximizable: false,
    skipTaskbar: true,
    hasShadow: false,
    focusable: false,
    acceptFirstMouse: true,
    backgroundColor: '#00000000',
    type: 'panel',
    webPreferences: {
      preload: join(__dirname, '../preload/index.js'),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: false,
      additionalArguments: ['--jaz-permission-window'],
    },
  })
  panel.setAlwaysOnTop(true, 'floating')
  panel.setVisibleOnAllWorkspaces(true, { visibleOnFullScreen: true, skipTransformProcessType: true })
  const tracker = spawn('/usr/bin/osascript', ['-l', 'JavaScript', '-e', TRACKER], { stdio: ['ignore', 'pipe', 'ignore'] })
  const lines = createInterface({ input: tracker.stdout })
  const own = (event: IpcMainEvent) => event.sender === panel.webContents && event.senderFrame === panel.webContents.mainFrame
  const drag = (event: IpcMainEvent) => {
    if (own(event)) {
      event.sender.startDrag({ file: resolve(process.execPath, '../../..'), icon: nativeImage.createFromPath(appIcon).resize({ width: 64 }) })
    }
  }
  const dismiss = (event: IpcMainEvent) => {
    if (own(event)) {
      finish()
    }
  }
  const poll = setInterval(() => {
    if (granted()) {
      finish()
    }
  }, 500)
  const abandoned = setTimeout(() => finish(), 10000)
  const finish = () => {
    if (close !== finish) {
      return
    }
    close = undefined
    clearInterval(poll)
    clearTimeout(abandoned)
    lines.close()
    tracker.kill()
    ipcMain.removeListener(PERMISSION_GUIDE_CHANNEL + 'drag', drag)
    ipcMain.removeListener(PERMISSION_GUIDE_CHANNEL + 'close', dismiss)
    if (!panel.isDestroyed()) {
      panel.destroy()
    }
  }
  close = finish
  ipcMain.on(PERMISSION_GUIDE_CHANNEL + 'drag', drag)
  ipcMain.on(PERMISSION_GUIDE_CHANNEL + 'close', dismiss)
  panel.once('closed', finish)
  tracker.once('error', finish)
  let side: boolean | undefined
  lines.on('line', (line) => {
    clearTimeout(abandoned)
    if (line === 'none') {
      finish()
      return
    }
    const [x, y, width, height, front] = line.split(' ').map(Number)
    if (!front) {
      panel.hide()
      return
    }
    const area = screen.getDisplayMatching({ x, y, width, height }).workArea
    if (side === undefined) {
      side = x + width + SIDE.width <= area.x + area.width
      const search = side ? 'side' : ''
      if (process.env.ELECTRON_RENDERER_URL) {
        void panel.loadURL(`${process.env.ELECTRON_RENDERER_URL}?${search}#${permission}`)
      } else {
        void panel.loadFile(join(__dirname, '../renderer/index.html'), { search, hash: permission })
      }
    }
    const bounds = side
      ? { width: SIDE.width, height: SIDE.height, x: x + width, y: y + LIST.firstRow - SIDE.row }
      : { width: Math.max(360, width - LIST.left - LIST.right + 2 * BELOW.margin), height: BELOW.height, x: x + LIST.left - BELOW.margin, y: y + height }
    panel.setBounds({
      ...bounds,
      x: Math.round(Math.min(Math.max(bounds.x, area.x), area.x + area.width - bounds.width)),
      y: Math.round(Math.min(Math.max(bounds.y, area.y), area.y + area.height - bounds.height)),
    })
    panel.showInactive()
  })
  await shell.openExternal(`x-apple.systempreferences:com.apple.preference.security?${PANES[permission]}`)
}
