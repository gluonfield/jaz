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
const SIZE = { width: 360, height: 76 }

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

// Opens the permission's System Settings list with a panel docked under the
// window: dragging Jaz from the panel into the list grants the permission.
export async function guidePermission(permission: ComputerPermission, granted: () => boolean): Promise<void> {
  close?.()
  const panel = new BrowserWindow({
    ...SIZE,
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
    panel.setBounds({
      ...SIZE,
      x: Math.round(Math.min(Math.max(x + (width - SIZE.width) / 2, area.x), area.x + area.width - SIZE.width)),
      y: Math.round(Math.min(y + height + 12, area.y + area.height - SIZE.height)),
    })
    panel.showInactive()
  })
  if (process.env.ELECTRON_RENDERER_URL) {
    void panel.loadURL(`${process.env.ELECTRON_RENDERER_URL}#${permission}`)
  } else {
    void panel.loadFile(join(__dirname, '../renderer/index.html'), { hash: permission })
  }
  await shell.openExternal(`x-apple.systempreferences:com.apple.preference.security?${PANES[permission]}`)
}
