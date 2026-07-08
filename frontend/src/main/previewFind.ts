import { type Input, type WebContents } from 'electron'
import { isPreviewURL } from '../shared/preview'
import { PREVIEW_FIND_SHORTCUT_CHANNEL } from '../shared/previewFind'

function isFindShortcut(input: Input): boolean {
  if (input.type !== 'keyDown' || input.shift || input.alt) return false
  if (!input.meta && !input.control) return false
  return input.key.toLowerCase() === 'f'
}

export function attachPreviewFindShortcuts(contents: WebContents): void {
  if (contents.getType() !== 'webview') return
  contents.on('before-input-event', (event, input) => {
    if (!isFindShortcut(input) || !isPreviewURL(contents.getURL())) return
    const host = contents.hostWebContents
    if (!host || host.isDestroyed()) return
    event.preventDefault()
    host.send(PREVIEW_FIND_SHORTCUT_CHANNEL)
  })
}
