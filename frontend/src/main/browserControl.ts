import { ipcMain, webContents } from 'electron'
import { BROWSER_COMMAND_METHODS, BROWSER_COMMAND_CHANNEL, type BrowserCommandRequest } from '@shared/browserControl'

export function installBrowserControl(): void {
  ipcMain.handle(BROWSER_COMMAND_CHANNEL, async (event, request: BrowserCommandRequest) => {
    const target = webContents.fromId(request.webContentsId)
    if (!target || target.isDestroyed() || target.getType() !== 'webview' || target.hostWebContents?.id !== event.sender.id) {
      throw new Error('Browser target must belong to this Jaz window')
    }
    if (!BROWSER_COMMAND_METHODS.has(request.method)) {
      throw new Error(`Unsupported browser command: ${request.method}`)
    }
    if (request.method === 'Jaz.cursorScale') {
      return target.getZoomFactor() / event.sender.getZoomFactor()
    }
    if (request.method === 'Page.navigate' || request.method === 'Jaz.navigate') {
      const url = new URL(String(request.params?.url))
      if (url.protocol !== 'http:' && url.protocol !== 'https:') {
        throw new Error('Browser navigation requires HTTP or HTTPS')
      }
      if (request.method === 'Jaz.navigate') {
        await target.loadURL(url.href)
        return {}
      }
    }
    if (!target.debugger.isAttached()) {
      target.debugger.attach('1.3')
    }
    if (request.method.startsWith('Input.')) {
      await target.debugger.sendCommand('Emulation.setFocusEmulationEnabled', { enabled: true })
    }
    return target.debugger.sendCommand(request.method, request.params)
  })
}
