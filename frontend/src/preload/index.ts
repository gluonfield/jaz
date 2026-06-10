import { contextBridge, ipcRenderer } from 'electron'

const apiBaseUrl = process.env['JAZ_API_URL'] ?? 'http://localhost:8080'

contextBridge.exposeInMainWorld('jaz', {
  apiBaseUrl,
  setNativeTheme: (source: 'light' | 'dark' | 'system') =>
    ipcRenderer.send('jaz:set-native-theme', source),
  startLocalBackend: (): Promise<{ ok: boolean; error?: string }> =>
    ipcRenderer.invoke('jaz:start-local-backend'),
})
