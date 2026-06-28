import { readFileSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'
import {
  BrowserWindow,
  Menu,
  type MenuItemConstructorOptions,
  type Rectangle,
  type WebContents,
  type WebPreferences,
  app,
  ipcMain,
  nativeTheme,
  session,
  shell,
  systemPreferences,
} from 'electron'
import appIcon from '../assets/jaz-icon-1024.png?asset'
import { isPreviewURL } from '../shared/preview'
import { startLocalBackend, stopLocalBackend } from './backend'
import { attachBrowserNavigationCommands, attachBrowserNavigationShortcuts } from './browserNavigation'
import { attachWindowLifecycle, installMainDiagnostics } from './diagnostics'
import { getDeviceIdentity, getDeviceMetadata } from './deviceIdentity'
import { canGrantAppPermission } from './permissions'
import { setupLauncher, teardownLauncher } from './spotlight'
import { createUpdateController } from './updater'

// Matches --color-bg under :root.dark; used as the window paint color before
// the renderer mounts so a dark launch doesn't flash white behind the content.
const DARK_BG = '#1d1f24'

const APP_NAME = 'Jaz'

app.setName(APP_NAME)
installMainDiagnostics()

let mainWindow: BrowserWindow | null = null
const updates = createUpdateController(() => mainWindow)
const previewURLTargets = new Set<number>()

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

function lockPreviewWebviewPreferences(webPreferences: WebPreferences): void {
  const prefs = webPreferences as WebPreferences & { preloadURL?: string }
  delete prefs.preload
  delete prefs.preloadURL
  webPreferences.nodeIntegration = false
  webPreferences.contextIsolation = true
  webPreferences.sandbox = true
  webPreferences.webSecurity = true
  webPreferences.allowRunningInsecureContent = false
}

function attachExternalOpenHandler(contents: WebContents): void {
  contents.setWindowOpenHandler(({ url }) => {
    if (contents.getType() === 'webview' && !isPreviewURL(url)) {
      return { action: 'deny' }
    }
    shell.openExternal(url)
    return { action: 'deny' }
  })
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

function attachPreviewNavigationGuard(contents: WebContents): void {
  contents.on('will-navigate', (event, url) => {
    if (contents.getType() !== 'webview' || isPreviewURL(url)) return
    event.preventDefault()
  })
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

function openPreviewURL(url: string, contents: WebContents): void {
  if (!canOpenPreviewURL(contents)) return
  const target = previewURLTarget(contents)
  const win = BrowserWindow.fromWebContents(target)
  if (!win || win.isDestroyed()) return
  win.show()
  win.focus()
  target.send('jaz:open-preview-url', url)
}

// Covers every window — main, preview webviews, and per-board windows.
app.on('web-contents-created', (_event, contents) => {
  contents.once('destroyed', () => previewURLTargets.delete(contents.id))
  attachBrowserNavigationShortcuts(contents)
  attachContextMenu(contents)
  attachExternalOpenHandler(contents)
  attachPreviewNavigationGuard(contents)
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
    trafficLightPosition: { x: 18, y: 18 },
    webPreferences: {
      preload: join(__dirname, '../preload/index.js'),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: false,
      webviewTag: true,
    },
  })

  mainWindow = win
  attachWindowLifecycle(win, { label: 'main', onDidFinishLoad: () => updates.sendStatusTo(win) })
  attachBrowserNavigationCommands(win)
  win.on('closed', () => {
    if (mainWindow === win) mainWindow = null
  })

  win.webContents.on('will-attach-webview', (event, webPreferences, params) => {
    if (!isPreviewURL(params.src)) {
      event.preventDefault()
      return
    }
    lockPreviewWebviewPreferences(webPreferences)
  })

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
  attachWindowLifecycle(win, { label: `board ${boardId}` })
  attachBrowserNavigationCommands(win)
  win.on('close', () => saveBoardBounds(boardId, win.getBounds()))
  win.on('closed', () => boardWindows.delete(boardId))
  win.webContents.setWindowOpenHandler(({ url }) => {
    shell.openExternal(url)
    return { action: 'deny' }
  })
  if (process.env['ELECTRON_RENDERER_URL']) {
    win.loadURL(`${process.env['ELECTRON_RENDERER_URL']}/boards/${boardId}`)
  } else {
    win.loadFile(join(__dirname, '../renderer/index.html'), { hash: `/boards/${boardId}` })
  }
}

app.whenReady().then(() => {
  installApplicationMenu()

  // Renderer mirrors its theme choice here so the native chrome (macOS traffic
  // lights, native scrollbars) and any new window's paint color match.
  ipcMain.on('jaz:set-native-theme', (_event, source) => {
    if (source === 'light' || source === 'dark' || source === 'system') {
      nativeTheme.themeSource = source
    }
  })

  ipcMain.handle('jaz:start-local-backend', () => startLocalBackend())
  ipcMain.handle('jaz:get-device-identity', () => getDeviceIdentity())
  ipcMain.handle('jaz:get-device-metadata', () => getDeviceMetadata())
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

  // Voice mode records from the mic; font settings enumerate installed fonts.
  // Grant both only to Jaz's own top-level renderer, not embedded frames.
  session.defaultSession.setPermissionRequestHandler((contents, permission, callback, details) => {
    callback(canGrantAppPermission(contents, permission, details))
  })
  // queryLocalFonts() also consults the synchronous permission check; grant the
  // same set so it doesn't fall back to a denied default.
  session.defaultSession.setPermissionCheckHandler((contents, permission, _origin, details) =>
    canGrantAppPermission(contents, permission, details),
  )
  if (process.platform === 'darwin') {
    app.dock?.setIcon(appIcon)
    app.setAboutPanelOptions({
      applicationName: APP_NAME,
      applicationVersion: app.getVersion(),
      iconPath: appIcon,
    })
    void systemPreferences.askForMediaAccess('microphone')
  }
  setupLauncher()
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
  teardownLauncher()
  stopLocalBackend()
})
