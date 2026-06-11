import { contextBridge, ipcRenderer } from 'electron'

const apiBaseUrl = process.env['JAZ_API_URL'] ?? 'http://localhost:5299'

// Board windows are spawned with this flag so the renderer can drop the app
// chrome (sidebar, titlebar) and render the board full-bleed.
const windowKind = process.argv.includes('--jaz-board-window') ? 'board' : 'main'

contextBridge.exposeInMainWorld('jaz', {
  apiBaseUrl,
  windowKind,
  setNativeTheme: (source: 'light' | 'dark' | 'system') =>
    ipcRenderer.send('jaz:set-native-theme', source),
  startLocalBackend: (): Promise<{ ok: boolean; error?: string }> =>
    ipcRenderer.invoke('jaz:start-local-backend'),
  openBoardWindow: (boardId: string) => ipcRenderer.send('jaz:open-board-window', boardId),
  // Board windows deep-link into the main app instead of navigating themselves.
  openInMain: (path: string) => ipcRenderer.send('jaz:open-in-main', path),
  onOpenRoute: (handler: (path: string) => void): (() => void) => {
    const listener = (_event: unknown, path: unknown): void => {
      if (typeof path === 'string') handler(path)
    }
    ipcRenderer.on('jaz:open-route', listener)
    return () => ipcRenderer.removeListener('jaz:open-route', listener)
  },
})
