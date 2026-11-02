import { app as electronApp } from 'electron'

export const app = {
  isPackaged: false,
  getPath: (name: Parameters<typeof electronApp.getPath>[0]) => electronApp.getPath(name),
}
export const powerSaveBlocker = {
  start: () => 1,
  stop: () => {},
  isStarted: () => true,
}
