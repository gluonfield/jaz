import { contextBridge, ipcRenderer, webUtils } from 'electron'
import { browserPasswords } from '@preload/browserPasswords'
import { browserDownloads } from '@preload/browserDownloads'
import { BROWSER_COMMAND_CHANNEL } from '@shared/browserControl'
import { BROWSER_PROFILE_CHANNELS, type BrowserProfileAPI } from '@shared/browserProfile'

contextBridge.exposeInMainWorld('jaz', {
  browserPasswords,
  browserDownloads,
  browserProfiles: {
    list: () => ipcRenderer.invoke(BROWSER_PROFILE_CHANNELS.list),
    import: (id, selection) => ipcRenderer.invoke(BROWSER_PROFILE_CHANNELS.import, id, selection),
  } satisfies BrowserProfileAPI,
  windowKind: 'main',
  pathForFile: (file: File) => webUtils.getPathForFile(file),
  openExternalURL: (url: string) => ipcRenderer.send('jaz:open-external-url', url),
  onOpenPreviewURL: (handler: (url: string) => void) => {
    const listener = (_event: unknown, url: string) => handler(url)
    ipcRenderer.on('jaz:open-preview-url', listener)
    ipcRenderer.send('jaz:set-preview-url-target-active', true)
    return () => {
      ipcRenderer.removeListener('jaz:open-preview-url', listener)
      ipcRenderer.send('jaz:set-preview-url-target-active', false)
    }
  },
  get apiBaseUrl() {
    return location.origin
  },
  browserCommand: (request: unknown) => ipcRenderer.invoke(BROWSER_COMMAND_CHANNEL, request),
})
contextBridge.exposeInMainWorld('smoke', {
  backend: () => ipcRenderer.invoke('smoke:backend'),
  browserExists: (id: number) => ipcRenderer.invoke('smoke:browser-exists', id),
  openedURLs: () => ipcRenderer.invoke('smoke:opened-urls'),
  popupURLs: () => ipcRenderer.invoke('smoke:popup-urls'),
  tabURLs: () => ipcRenderer.invoke('smoke:tab-urls'),
  passwordStore: () => ipcRenderer.invoke('smoke:password-store'),
  pointer: (type: string, x: number, y: number, button?: 'left' | 'right') => ipcRenderer.invoke('smoke:pointer', type, x, y, button),
  key: (key: string, modifiers?: string[]) => ipcRenderer.invoke('smoke:key', key, modifiers),
  capture: (name?: string) => ipcRenderer.invoke('smoke:capture', name),
  resize: (width: number, height: number) => ipcRenderer.invoke('smoke:resize', width, height),
  dropFolder: (x: number, y: number) => ipcRenderer.invoke('smoke:drop-folder', x, y),
  result: (result: unknown) => ipcRenderer.send('smoke:result', result),
})
