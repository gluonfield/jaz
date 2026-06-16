import { contextBridge, ipcRenderer } from 'electron'
import type { UpdateStatus } from '../shared/update'

const apiBaseUrl = process.env['JAZ_API_URL'] ?? 'http://localhost:5299'

// Board windows are spawned with this flag so the renderer can drop the app
// chrome (sidebar, titlebar) and render the board full-bleed.
const windowKind = process.argv.includes('--jaz-board-window') ? 'board' : 'main'
let previewURLTargetSubscriptions = 0

contextBridge.exposeInMainWorld('jaz', {
  apiBaseUrl,
  windowKind,
  setNativeTheme: (source: 'light' | 'dark' | 'system') =>
    ipcRenderer.send('jaz:set-native-theme', source),
  startLocalBackend: (): Promise<{ ok: boolean; url?: string; key?: string; error?: string }> =>
    ipcRenderer.invoke('jaz:start-local-backend'),
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
  // Board windows deep-link into the main app instead of navigating themselves.
  openInMain: (path: string) => ipcRenderer.send('jaz:open-in-main', path),
  onOpenSideBrowserURL: (handler: (url: string) => void): (() => void) => {
    const listener = (_event: unknown, url: unknown): void => {
      if (typeof url === 'string') handler(url)
    }
    ipcRenderer.on('jaz:open-side-browser-url', listener)
    return () => ipcRenderer.removeListener('jaz:open-side-browser-url', listener)
  },
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
})
