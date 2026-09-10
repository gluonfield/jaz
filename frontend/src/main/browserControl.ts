import { ipcMain, webContents, type WebContents } from 'electron'
import { BROWSER_COMMAND_METHODS, BROWSER_COMMAND_CHANNEL, type BrowserCommandRequest } from '@shared/browserControl'
import { BrowserFrames } from '@main/browserFrames'

export function installBrowserControl(): void {
  const frames = new WeakMap<WebContents, BrowserFrames>()
  ipcMain.handle(BROWSER_COMMAND_CHANNEL, async (event, request: BrowserCommandRequest) => {
    const target = webContents.fromId(request.webContentsId)
    if (!target || target.isDestroyed() || target.getType() !== 'webview' || target.hostWebContents?.id !== event.sender.id) {
      throw new Error('Browser target must belong to this Jaz window')
    }
    if (!BROWSER_COMMAND_METHODS.has(request.method)) {
      throw new Error(`Unsupported browser command: ${request.method}`)
    }
    let ownedFrames = frames.get(target)
    if (!ownedFrames) {
      ownedFrames = new BrowserFrames(target)
      frames.set(target, ownedFrames)
    }
    if (request.sessionId && !ownedFrames.owns(request.sessionId)) {
      throw new Error('Browser debugger session must belong to this webview')
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
    if (request.method === 'Jaz.frames') {
      return ownedFrames.list()
    }
    if (request.method.startsWith('Input.')) {
      await target.debugger.sendCommand('Emulation.setFocusEmulationEnabled', { enabled: true })
    }
    return target.debugger.sendCommand(request.method, request.params, request.sessionId || undefined)
  })
}
