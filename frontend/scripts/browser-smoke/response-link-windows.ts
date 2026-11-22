import { BrowserWindow } from 'electron'
import { join } from 'node:path'

export async function exerciseResponseLinks(origin: string, external: string[]) {
  for (const desktop of [true, false]) {
    const window = new BrowserWindow({ show: false, webPreferences: {
      partition: desktop ? 'reply-links-desktop' : 'reply-links-web',
      preload: desktop ? join(process.env.JAZ_BROWSER_SMOKE_DIR!, 'preload.js') : undefined,
      contextIsolation: true, sandbox: true,
    } })
    const tabs: string[] = []
    window.webContents.setWindowOpenHandler(({ url }) => { tabs.push(url); return { action: 'deny' } })
    try {
      for (const reload of [false, true]) {
        await window.loadURL(origin + '/?suite=response-links')
        const expected: string[] = await window.webContents.executeJavaScript(`window.replyLinksReady.then(check => check(${reload}))`, true)
        const native = external.splice(0)
        const opened = tabs.splice(0)
        if (native.join() !== (desktop ? expected : []).join() || opened.join() !== (desktop ? [] : expected).join()) {
          throw new Error('Unexpected reply link destinations: ' + JSON.stringify({ desktop, reload, native, opened, expected }))
        }
      }
    } finally {
      window.destroy()
    }
  }
}
