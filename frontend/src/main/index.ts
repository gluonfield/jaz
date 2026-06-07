import { join } from 'node:path'
import { BrowserWindow, app, session, shell, systemPreferences } from 'electron'
import appIcon from '../assets/jaz-icon-1024.png?asset'

const APP_NAME = 'Jaz'

app.setName(APP_NAME)

function createWindow(): void {
  const win = new BrowserWindow({
    title: APP_NAME,
    width: 1280,
    height: 832,
    minWidth: 940,
    minHeight: 600,
    show: false,
    icon: appIcon,
    backgroundColor: '#ffffff',
    titleBarStyle: process.platform === 'darwin' ? 'hiddenInset' : 'default',
    trafficLightPosition: { x: 18, y: 18 },
    webPreferences: {
      preload: join(__dirname, '../preload/index.js'),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: false,
    },
  })

  win.once('ready-to-show', () => win.show())

  // External links open in the system browser, never in the app.
  win.webContents.setWindowOpenHandler(({ url }) => {
    shell.openExternal(url)
    return { action: 'deny' }
  })

  if (process.env['ELECTRON_RENDERER_URL']) {
    win.loadURL(process.env['ELECTRON_RENDERER_URL'])
  } else {
    win.loadFile(join(__dirname, '../renderer/index.html'))
  }
}

app.whenReady().then(() => {
  // Voice mode records from the mic; allow media (macOS still shows its own
  // TCC prompt) and deny everything else.
  session.defaultSession.setPermissionRequestHandler((_wc, permission, callback) => {
    callback(permission === 'media')
  })
  if (process.platform === 'darwin') {
    app.dock?.setIcon(appIcon)
    app.setAboutPanelOptions({
      applicationName: APP_NAME,
      applicationVersion: app.getVersion(),
      iconPath: appIcon,
    })
    void systemPreferences.askForMediaAccess('microphone')
  }
  createWindow()
  app.on('activate', () => {
    if (BrowserWindow.getAllWindows().length === 0) createWindow()
  })
})

app.on('window-all-closed', () => {
  if (process.platform !== 'darwin') app.quit()
})
