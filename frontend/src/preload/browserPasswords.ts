import { ipcRenderer } from 'electron'
import { PASSWORD_CHANNEL, PASSWORD_CHANGED_CHANNEL, type BrowserPasswordAPI } from '@shared/browserPasswords'

export const browserPasswords: BrowserPasswordAPI = {
  state: (id) => ipcRenderer.invoke(PASSWORD_CHANNEL, id),
  act: (id, action) => ipcRenderer.invoke(PASSWORD_CHANNEL, id, action),
  subscribe: (id, handler) => {
    const listener = (_event: unknown, target: number) => {
      if (target === id) {
        handler()
      }
    }
    ipcRenderer.on(PASSWORD_CHANGED_CHANNEL, listener)
    return () => ipcRenderer.removeListener(PASSWORD_CHANGED_CHANNEL, listener)
  },
}
