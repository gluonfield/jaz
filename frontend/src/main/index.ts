import { readFileSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'
import {
  BrowserWindow,
  Menu,
  type MenuItemConstructorOptions,
  type Rectangle,
  type WebContents,
  app,
  ipcMain,
  nativeTheme,
  session,
  shell,
} from 'electron'
import appIcon from '../assets/jaz-icon-1024.png?asset'
import { isPreviewURL } from '../shared/preview'
import { startLocalBackend, stopLocalBackend } from './backend'
import { attachBrowserNavigationCommands, attachBrowserNavigationShortcuts } from '@main/browserNavigation'
import { attachWindowOpenHandler } from '@main/browserPopups'
import { attachWindowLifecycle, installMainDiagnostics } from './diagnostics'
import { installCredentialEncryption } from './credentialEncryption'
import { getDeviceIdentity, getDeviceMetadata } from './deviceIdentity'
import { registerDictation } from './dictation'
import { configureAppPermissions } from './permissions'
import { createThreadNotificationMonitor } from './notifications'
import { attachPreviewFindShortcuts } from './previewFind'
import { installComputerControl } from '@main/computerControl'
import { installSystemPermissions } from '@main/systemPermissions'
import { installBrowserControl } from '@main/browserControl'
import { installBrowserProfileImport } from '@main/browserProfileImport'
import { attachPreviewWebviews, configurePreviewSession } from '@main/previewSession'
import { installBrowserPasswords } from '@main/browserPasswords'
import { installBrowserDownloads } from '@main/browserDownloads'
import { setupLauncher, teardownLauncher } from './spotlight'
import { attachVoiceOverlay } from './voiceOverlay'
import { createUpdateController } from './updater'
import { createConnectionSession } from '@main/connectionSession'

// Matches --color-bg under :root.dark; used as the window paint color before
// the renderer mounts so a dark launch doesn't flash white behind the content.
const DARK_BG = '#141517'

const APP_NAME = 'Jaz'

app.setName(APP_NAME)
app.setAppUserModelId('dev.wins.jaz')
installMainDiagnostics()
installBrowserControl()
installComputerControl()
if (process.platform === 'darwin') {
  installSystemPermissions()
}
installBrowserProfileImport()
installBrowserPasswords()

let mainWindow: BrowserWindow | null = null
const updates = createUpdateController(() => mainWindow)
const previewURLTargets = new Set<number>()
const threadNotifications = createThreadNotificationMonitor(openInMain)
const connectionSession = createConnectionSession({
  rendererPath: join(__dirname, '../renderer/index.html'),
  startLocal: startLocalBackend,
  profile: () => ({ ...getDeviceIdentity(), ...getDeviceMetadata() }),
  stopConsumers: () => threadNotifications.stop(),
})

function installApplicationMenu(): void {
  if (process.platform !== 'darwin') return
  const template: MenuItemConstructorOptions[] = [
    {
      label: APP_NAME,
      submenu: [
        { role: 'about', label: `About ${APP_NAME}` },
        {
          label: 'Check for Updates...',
          enabled: app.isPackaged,
          click: () => updates.checkForUpdates(),
        },
        { type: 'separator' },
        { role: 'services' },
        { type: 'separator' },
        { role: 'hide', label: `Hide ${APP_NAME}` },
        { role: 'hideOthers' },
        { role: 'unhide' },
        { type: 'separator' },
        { role: 'quit', label: `Quit ${APP_NAME}` },
      ],
    },
    {
      label: 'Edit',
      submenu: [
        { role: 'undo' },
        { role: 'redo' },
        { type: 'separator' },
        { role: 'cut' },
        { role: 'copy' },
        { role: 'paste' },
        { role: 'pasteAndMatchStyle' },
        { role: 'delete' },
        { role: 'selectAll' },
      ],
    },
    {
      label: 'View',
      submenu: [
        { role: 'reload' },
        { role: 'forceReload' },
        { role: 'toggleDevTools' },
        { type: 'separator' },
        { role: 'resetZoom' },
        { role: 'zoomIn' },
        { role: 'zoomOut' },
        { type: 'separator' },
        { role: 'togglefullscreen' },
      ],
    },
    {
      label: 'Window',
      submenu: [{ role: 'minimize' }, { role: 'zoom' }, { type: 'separator' }, { role: 'front' }],
    },
  ]
  Menu.setApplicationMenu(Menu.buildFromTemplate(template))
}

function openExternalURL(url: string): void {
  try {
    const parsed = new URL(url)
    if (parsed.protocol === 'http:' || parsed.protocol === 'https:') {
      shell.openExternal(parsed.toString())
    }
  } catch {
    // Ignore malformed renderer input.
  }
}

// Electron ships no default right-click menu; build the link and standard text
// actions wherever the click lands on a link, editable field, or text selection.
function attachContextMenu(contents: WebContents): void {
  contents.on('context-menu', (_event, params) => {
    const linkURL = params.linkURL.trim()
    const previewLinkURL = isPreviewURL(linkURL) ? linkURL : ''
    const canOpenSideBrowser = previewLinkURL !== '' && canOpenPreviewURL(contents)
    const editable = params.isEditable
    const selection = params.selectionText.trim()
    if (!previewLinkURL && !editable && selection === '') return

    const items: MenuItemConstructorOptions[] = []
    if (previewLinkURL) {
      if (canOpenSideBrowser) {
        items.push({
          label: 'Open in Side Browser',
          click: () => openPreviewURL(previewLinkURL, contents),
        })
      }
      items.push(
        {
          label: 'Open in Browser',
          click: () => shell.openExternal(previewLinkURL),
        },
      )
      if (editable || selection !== '') items.push({ type: 'separator' })
    }
    if (editable && params.misspelledWord) {
      for (const suggestion of params.dictionarySuggestions.slice(0, 5)) {
        items.push({ label: suggestion, click: () => contents.replaceMisspelling(suggestion) })
      }
      items.push(
        {
          label: 'Add to Dictionary',
          click: () => contents.session.addWordToSpellCheckerDictionary(params.misspelledWord),
        },
        { type: 'separator' },
      )
    }
    if (selection !== '') {
      const preview = selection.length > 30 ? `${selection.slice(0, 30)}…` : selection
      if (process.platform === 'darwin') {
        items.push({
          label: `Look Up “${preview}”`,
          click: () => contents.showDefinitionForSelection(),
        })
      }
      items.push(
        {
          label: `Search with Google “${preview}”`,
          click: () =>
            shell.openExternal(`https://www.google.com/search?q=${encodeURIComponent(selection)}`),
        },
        { type: 'separator' },
      )
    }
    if (editable) {
      items.push(
        { role: 'cut', enabled: params.editFlags.canCut },
        { role: 'copy', enabled: params.editFlags.canCopy },
        { role: 'paste', enabled: params.editFlags.canPaste },
        { role: 'selectAll', enabled: params.editFlags.canSelectAll },
      )
    } else {
      items.push({ role: 'copy', enabled: params.editFlags.canCopy })
    }
    Menu.buildFromTemplate(items).popup({
      window: BrowserWindow.fromWebContents(contents) ?? undefined,
    })
  })
}

function previewURLTarget(contents: WebContents): WebContents {
  return contents.hostWebContents ?? contents
}

function canOpenPreviewURL(contents: WebContents): boolean {
  const target = previewURLTarget(contents)
  return !target.isDestroyed() && previewURLTargets.has(target.id)
}

function openPreviewURL(url: string, contents: WebContents): boolean {
  if (!canOpenPreviewURL(contents)) return false
  const target = previewURLTarget(contents)
  const win = BrowserWindow.fromWebContents(target)
  if (!win || win.isDestroyed()) return false
  win.show()
  win.focus()
  target.send('jaz:open-preview-url', url)
  return true
}

// Covers every window — main, preview webviews, and per-board windows.
app.on('web-contents-created', (_event, contents) => {
  contents.once('destroyed', () => previewURLTargets.delete(contents.id))
  attachBrowserNavigationShortcuts(contents)
  attachPreviewFindShortcuts(contents)
  attachContextMenu(contents)
  attachWindowOpenHandler(contents, shell.openExternal, (url) => openPreviewURL(url, contents))
})

function createWindow(): void {
  const mac = process.platform === 'darwin'
  const win = new BrowserWindow({
    title: APP_NAME,
    width: 1280,
    height: 832,
    minWidth: 940,
    minHeight: 600,
    show: false,
    icon: appIcon,
    // On macOS the window paints the native sidebar material; the renderer
    // keeps the content column opaque and lets the sidebar show through.
    backgroundColor: mac ? '#00000000' : nativeTheme.shouldUseDarkColors ? DARK_BG : '#ffffff',
    ...(mac ? { vibrancy: 'sidebar' as const, visualEffectState: 'followWindow' as const } : {}),
    titleBarStyle: mac ? 'hiddenInset' : 'default',
    trafficLightPosition: { x: 16, y: 13 },
    webPreferences: {
      preload: join(__dirname, '../preload/index.js'),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: false,
      webviewTag: true,
    },
  })

  mainWindow = win
  connectionSession.attach(win, true)
  attachVoiceOverlay(win)
  attachWindowLifecycle(win, { label: 'main', onDidFinishLoad: () => updates.sendStatusTo(win) })
  attachBrowserNavigationCommands(win)
  win.on('closed', () => {
    if (mainWindow === win) mainWindow = null
  })

  attachPreviewWebviews(win.webContents, join(__dirname, '../preload/index.js'))

  if (process.env['ELECTRON_RENDERER_URL']) {
    win.loadURL(process.env['ELECTRON_RENDERER_URL'])
  } else {
    win.loadFile(join(__dirname, '../renderer/index.html'))
  }
}

// Deep link from a board window into the main app (e.g. "Open loop").
function openInMain(path: string): void {
  if (mainWindow && !mainWindow.isDestroyed()) {
    mainWindow.show()
    mainWindow.focus()
    mainWindow.webContents.send('jaz:open-route', path)
    return
  }
  createWindow()
  mainWindow?.webContents.once('did-finish-load', () => {
    mainWindow?.webContents.send('jaz:open-route', path)
  })
}

// Board windows: one OS window per board, bounds remembered locally (they are
// device-specific, unlike the board's tile layout which lives in the backend).
const boardWindows = new Map<string, BrowserWindow>()
const boardBoundsFile = (): string => join(app.getPath('userData'), 'board-windows.json')

function loadBoardBounds(): Record<string, Rectangle> {
  try {
    return JSON.parse(readFileSync(boardBoundsFile(), 'utf8')) as Record<string, Rectangle>
  } catch {
    return {}
  }
}

function saveBoardBounds(boardId: string, bounds: Rectangle): void {
  try {
    writeFileSync(boardBoundsFile(), JSON.stringify({ ...loadBoardBounds(), [boardId]: bounds }))
  } catch {
    // bounds are a convenience; never fail the close over them
  }
}

function openBoardWindow(boardId: string): void {
  const existing = boardWindows.get(boardId)
  if (existing && !existing.isDestroyed()) {
    existing.show()
    existing.focus()
    return
  }
  const saved = loadBoardBounds()[boardId]
  const win = new BrowserWindow({
    title: 'Board — Jaz',
    width: saved?.width ?? 960,
    height: saved?.height ?? 640,
    ...(saved ? { x: saved.x, y: saved.y } : {}),
    minWidth: 420,
    minHeight: 320,
    show: false,
    icon: appIcon,
    backgroundColor: nativeTheme.shouldUseDarkColors ? DARK_BG : '#ffffff',
    webPreferences: {
      preload: join(__dirname, '../preload/index.js'),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: false,
      additionalArguments: ['--jaz-board-window'],
    },
  })
  boardWindows.set(boardId, win)
  connectionSession.attach(win)
  attachWindowLifecycle(win, { label: `board ${boardId}` })
  attachBrowserNavigationCommands(win)
  win.on('close', () => saveBoardBounds(boardId, win.getBounds()))
  win.on('closed', () => boardWindows.delete(boardId))
  if (process.env['ELECTRON_RENDERER_URL']) {
    win.loadURL(`${process.env['ELECTRON_RENDERER_URL']}/boards/${boardId}`)
  } else {
    win.loadFile(join(__dirname, '../renderer/index.html'), { hash: `/boards/${boardId}` })
  }
}

app.whenReady().then(() => {
  configurePreviewSession()
  installBrowserDownloads()
  installApplicationMenu()
  registerDictation()

  // Renderer mirrors its theme choice here so the native chrome (macOS traffic
  // lights, native scrollbars) and any new window's paint color match.
  ipcMain.on('jaz:set-native-theme', (_event, source) => {
    if (source === 'light' || source === 'dark' || source === 'system') {
      nativeTheme.themeSource = source
    }
  })

  // The traffic lights live above the thread list; the renderer hides them
  // while the list is collapsed rather than moving them around.
  ipcMain.on('jaz:set-window-buttons-visible', (event, visible) => {
    const window = BrowserWindow.fromWebContents(event.sender)
    if (process.platform === 'darwin' && window && typeof visible === 'boolean') {
      window.setWindowButtonVisibility(visible)
    }
  })

  installCredentialEncryption()
  ipcMain.handle('jaz:start-local-backend', (event) => {
    const record = connectionSession.authorizeOwner(event)
    const document = record.document
    return startLocalBackend().then((result) => {
      connectionSession.authorizeOwner(event, document)
      return result
    })
  })
  ipcMain.handle('jaz:get-device-identity', () => getDeviceIdentity())
  ipcMain.handle('jaz:get-device-metadata', () => getDeviceMetadata())
  ipcMain.handle('jaz:configure-thread-notifications', (event, config) => {
    connectionSession.authorizeOwner(event)
    const active = connectionSession.current()
    if (config?.enabled && (active.phase !== 'connected' || config.baseUrl !== active.url || config.token !== active.session?.token)) {
      return false
    }
    return threadNotifications.configure(config)
  })
  updates.registerIpc()

  ipcMain.on('jaz:open-board-window', (_event, boardId) => {
    if (typeof boardId === 'string' && boardId !== '') openBoardWindow(boardId)
  })

  ipcMain.on('jaz:open-external-url', (_event, url) => {
    if (typeof url === 'string') openExternalURL(url)
  })

  ipcMain.on('jaz:open-in-main', (_event, path) => {
    if (typeof path === 'string' && path.startsWith('/')) openInMain(path)
  })

  ipcMain.on('jaz:set-preview-url-target-active', (event, active) => {
    if (active === true) previewURLTargets.add(event.sender.id)
    else previewURLTargets.delete(event.sender.id)
  })

  configureAppPermissions(session.defaultSession)
  if (process.platform === 'darwin') {
    // Development runs inside Electron.app, so it borrows the Jaz icon. Packaged builds keep the bundle icon,
    // which follows the system icon style.
    if (!app.isPackaged) {
      app.dock?.setIcon(appIcon)
    }
    app.setAboutPanelOptions({
      applicationName: APP_NAME,
      applicationVersion: app.getVersion(),
    })
  }
  setupLauncher((window) => connectionSession.attach(window))
  createWindow()
  updates.start()
  app.on('activate', () => {
    if (!mainWindow || mainWindow.isDestroyed()) {
      createWindow()
      return
    }
    mainWindow.show()
    mainWindow.focus()
  })
})

app.on('window-all-closed', () => {
  if (process.platform !== 'darwin') app.quit()
})

app.on('before-quit', () => {
  threadNotifications.stop()
  teardownLauncher()
  stopLocalBackend('before_quit')
})
