import { app, ipcMain, type WebContents } from 'electron'
import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { COMPUTER_CHANNEL, type ComputerAction, type ComputerStatus } from '@shared/computerControl'
import { ComputerRuntime } from '@main/computerRuntime'
import { trustedWindow } from '@main/permissions'

const loadDriver = () => import('@trycua/cua-driver')

async function availability(): Promise<ComputerStatus> {
  const status: ComputerStatus = { available: false, platform: process.platform }
  if (process.platform !== 'darwin' && process.platform !== 'win32') {
    return { ...status, reason: 'Computer use is available on macOS and Windows.' }
  }
  try {
    const sdk = await loadDriver()
    status.driverVersion = JSON.parse(readFileSync(join(app.getAppPath(), 'package.json'), 'utf8')).dependencies['@trycua/cua-driver']
    if (process.platform === 'darwin') {
      status.permissions = sdk.currentMacOsPermissionStatus()
      if (!status.permissions.accessibility || !status.permissions.screenRecording) {
        return { ...status, reason: 'Allow Accessibility and Screen Recording in Computer Use settings.' }
      }
    }
    return { ...status, available: true }
  } catch (error) {
    return { ...status, reason: 'Computer driver unavailable: ' + (error instanceof Error ? error.message : String(error)) }
  }
}

export function installComputerControl(): void {
  const runtime = new ComputerRuntime(async () => (await loadDriver()).CuaDriver.create(undefined), availability)
  const watching = new WeakSet<WebContents>()
  ipcMain.handle(COMPUTER_CHANNEL + 'status', (event) => {
    trustedWindow(event)
    return runtime.status()
  })
  ipcMain.handle(COMPUTER_CHANNEL + 'begin', (event, id: string, session: string) => {
    const owner = trustedWindow(event)
    id = commandID(id)
    if (typeof session !== 'string' || session.length > 256) {
      throw new Error('Invalid computer session')
    }
    if (!watching.has(event.sender)) {
      watching.add(event.sender)
      const cancel = () => void runtime.cancelOwner(owner).catch(console.error)
      event.sender.once('destroyed', cancel)
      event.sender.on('render-process-gone', cancel)
      event.sender.on('did-start-navigation', (_event, _url, inPlace, mainFrame) => {
        if (mainFrame && !inPlace) {
          cancel()
        }
      })
    }
    return runtime.begin(owner, id, session)
  })
  ipcMain.handle(COMPUTER_CHANNEL + 'call', (event, id: string, action: ComputerAction) => {
    const owner = trustedWindow(event)
    id = commandID(id)
    if (!action || typeof action !== 'object' || (action.name !== undefined && (typeof action.name !== 'string' || action.name.length > 100)) ||
      (action.args !== undefined && (!action.args || typeof action.args !== 'object' || Array.isArray(action.args))) ||
      JSON.stringify(action).length > 150000) {
      throw new Error('Invalid computer tool arguments')
    }
    return runtime.call(owner, id, action)
  })
  for (const method of ['end', 'cancel'] as const) {
    ipcMain.handle(COMPUTER_CHANNEL + method, (event, id: string) => runtime[method](trustedWindow(event), commandID(id)))
  }
  let stopped = false
  app.on('before-quit', (event) => {
    if (stopped) {
      return
    }
    event.preventDefault()
    void runtime.shutdown().catch(console.error).finally(() => {
      stopped = true
      app.quit()
    })
  })
}

function commandID(value: unknown): string {
  if (typeof value !== 'string' || !/^[a-zA-Z0-9-]{1,128}$/.test(value)) {
    throw new Error('Invalid computer command ID')
  }
  return value
}
