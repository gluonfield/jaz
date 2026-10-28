import { join } from 'node:path'
import { pathToFileURL } from 'node:url'
import { app } from 'electron'

export function loadComputerDriver(): Promise<typeof import('@trycua/cua-driver')> {
  // The SDK resolves its dylib relative to its own entry; dlopen cannot read ASAR paths.
  return app.isPackaged
    ? import(pathToFileURL(join(process.resourcesPath, 'app.asar.unpacked/node_modules/@trycua/cua-driver/dist/index.js')).href)
    : import('@trycua/cua-driver')
}
