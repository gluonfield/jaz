import type { WebContents } from 'electron'
import { BROWSER_CLOSE_SHORTCUT_CHANNEL } from '@shared/browserCloseShortcut'

export function attachBrowserCloseShortcut(contents: WebContents): void {
  if (contents.getType() !== 'webview') return
  contents.on('before-input-event', (event, input) => {
    if (input.type !== 'keyDown' || !input.meta || input.control || input.alt || input.shift || input.isAutoRepeat || input.key.toLowerCase() !== 'w') return
    const host = contents.hostWebContents
    if (!host || host.isDestroyed()) return
    event.preventDefault()
    host.send(BROWSER_CLOSE_SHORTCUT_CHANNEL)
  })
}
