import { contextBridge, ipcRenderer } from 'electron'
import {
  BROWSER_NAVIGATION_CHANNEL,
  type BrowserNavigationDirection,
} from '../shared/browserNavigation'
import { PREVIEW_FIND_SHORTCUT_CHANNEL } from '../shared/previewFind'
import type { ThreadNotificationConfig } from '../shared/notifications'
import type { UpdateStatus } from '../shared/update'

const apiBaseUrl = process.env['JAZ_API_URL'] ?? 'http://127.0.0.1:5299'

// Board and launcher windows are spawned with a flag so the renderer can drop
// the app chrome (sidebar, titlebar) and render that surface full-bleed.
const windowKind = process.argv.includes('--jaz-board-window')
  ? 'board'
  : process.argv.includes('--jaz-launcher-window')
    ? 'launcher'
    : 'main'
let previewURLTargetSubscriptions = 0

contextBridge.exposeInMainWorld('jaz', {
  apiBaseUrl,
  windowKind,
  setNativeTheme: (source: 'light' | 'dark' | 'system') =>
    ipcRenderer.send('jaz:set-native-theme', source),
  startLocalBackend: (): Promise<{ ok: boolean; url?: string; key?: string; error?: string }> =>
    ipcRenderer.invoke('jaz:start-local-backend'),
  getDeviceIdentity: (): Promise<{ device_id: string; public_key: string }> =>
    ipcRenderer.invoke('jaz:get-device-identity'),
  getDeviceMetadata: (): Promise<{
    name: string
    platform: string
    device_family: string
    model_identifier: string
    app_version: string
  }> => ipcRenderer.invoke('jaz:get-device-metadata'),
  configureThreadNotifications: (config: ThreadNotificationConfig): Promise<boolean> =>
    ipcRenderer.invoke('jaz:configure-thread-notifications', config),
  getUpdateStatus: (): Promise<UpdateStatus> => ipcRenderer.invoke('jaz:get-update-status'),
  installUpdate: (): Promise<{ ok: boolean; error?: string }> =>
    ipcRenderer.invoke('jaz:install-update'),
  onUpdateStatus: (handler: (status: UpdateStatus) => void): (() => void) => {
    const listener = (_event: unknown, status: unknown): void => {
      if (
        typeof status === 'object' &&
        status !== null &&
        'state' in status &&
        typeof status.state === 'string'
      ) {
        handler(status as UpdateStatus)
      }
    }
    ipcRenderer.on('jaz:update-status', listener)
    return () => ipcRenderer.removeListener('jaz:update-status', listener)
  },
  openBoardWindow: (boardId: string) => ipcRenderer.send('jaz:open-board-window', boardId),
  openExternalURL: (url: string) => ipcRenderer.send('jaz:open-external-url', url),
  captureScreenRect: (rect: {
    x: number
    y: number
    width: number
    height: number
  }): Promise<{ ok: boolean; data?: string; denied?: boolean }> =>
    ipcRenderer.invoke('jaz:capture-screen-rect', rect),
  hideLauncher: () => ipcRenderer.send('jaz:hide-launcher'),
  onLauncherShown: (handler: () => void): (() => void) => {
    const listener = (): void => handler()
    ipcRenderer.on('jaz:launcher-shown', listener)
    return () => ipcRenderer.removeListener('jaz:launcher-shown', listener)
  },
  // Board windows deep-link into the main app instead of navigating themselves.
  openInMain: (path: string) => ipcRenderer.send('jaz:open-in-main', path),
  onOpenRoute: (handler: (path: string) => void): (() => void) => {
    const listener = (_event: unknown, path: unknown): void => {
      if (typeof path === 'string') handler(path)
    }
    ipcRenderer.on('jaz:open-route', listener)
    return () => ipcRenderer.removeListener('jaz:open-route', listener)
  },
  onOpenPreviewURL: (handler: (url: string) => void): (() => void) => {
    const listener = (_event: unknown, url: unknown): void => {
      if (typeof url === 'string') handler(url)
    }
    ipcRenderer.on('jaz:open-preview-url', listener)
    previewURLTargetSubscriptions += 1
    if (previewURLTargetSubscriptions === 1) {
      ipcRenderer.send('jaz:set-preview-url-target-active', true)
    }
    return () => {
      ipcRenderer.removeListener('jaz:open-preview-url', listener)
      previewURLTargetSubscriptions = Math.max(0, previewURLTargetSubscriptions - 1)
      if (previewURLTargetSubscriptions === 0) {
        ipcRenderer.send('jaz:set-preview-url-target-active', false)
      }
    }
  },
  onBrowserNavigation: (handler: (direction: BrowserNavigationDirection) => void): (() => void) => {
    const listener = (_event: unknown, direction: unknown): void => {
      if (direction === 'back' || direction === 'forward') handler(direction)
    }
    ipcRenderer.on(BROWSER_NAVIGATION_CHANNEL, listener)
    return () => ipcRenderer.removeListener(BROWSER_NAVIGATION_CHANNEL, listener)
  },
  onPreviewFindShortcut: (handler: () => void): (() => void) => {
    const listener = (): void => handler()
    ipcRenderer.on(PREVIEW_FIND_SHORTCUT_CHANNEL, listener)
    return () => ipcRenderer.removeListener(PREVIEW_FIND_SHORTCUT_CHANNEL, listener)
  },
})
