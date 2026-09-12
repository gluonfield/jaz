import { contextBridge, ipcRenderer } from 'electron'
import { browserPasswords } from '@preload/browserPasswords'
import { BROWSER_COMMAND_CHANNEL } from '@shared/browserControl'
import { BROWSER_PROFILE_CHANNELS, type BrowserProfileAPI } from '@shared/browserProfile'

contextBridge.exposeInMainWorld('jaz', {
  browserPasswords,
  browserProfiles: {
    dismissed: () => ipcRenderer.invoke(BROWSER_PROFILE_CHANNELS.dismissed),
    dismiss: () => ipcRenderer.invoke(BROWSER_PROFILE_CHANNELS.dismiss),
    list: () => ipcRenderer.invoke(BROWSER_PROFILE_CHANNELS.list),
    sites: (id) => ipcRenderer.invoke(BROWSER_PROFILE_CHANNELS.sites, id),
    import: (id, domains) => ipcRenderer.invoke(BROWSER_PROFILE_CHANNELS.import, id, domains),
  } satisfies BrowserProfileAPI,
  windowKind: 'main',
  get apiBaseUrl() {
    return location.origin
  },
  browserCommand: (request: unknown) => ipcRenderer.invoke(BROWSER_COMMAND_CHANNEL, request),
})
contextBridge.exposeInMainWorld('smoke', {
  backend: () => ipcRenderer.invoke('smoke:backend'),
  browserExists: (id: number) => ipcRenderer.invoke('smoke:browser-exists', id),
  passwordStore: () => ipcRenderer.invoke('smoke:password-store'),
  pointer: (type: string, x: number, y: number) => ipcRenderer.invoke('smoke:pointer', type, x, y),
  capture: (name?: string) => ipcRenderer.invoke('smoke:capture', name),
  resize: (width: number, height: number) => ipcRenderer.invoke('smoke:resize', width, height),
  result: (result: unknown) => ipcRenderer.send('smoke:result', result),
})
