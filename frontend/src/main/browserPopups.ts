import { session, shell, type Event, type WebContents } from 'electron'
import { PREVIEW_WEB_PREFERENCES, registerPreviewPopupOpener } from '@main/previewSession'
import { PREVIEW_PARTITION, isPreviewURL } from '@shared/preview'

function canNavigate(url: string): boolean {
  return url === 'about:blank' || isPreviewURL(url)
}

export function attachWindowOpenHandler(
  contents: WebContents,
  openExternal = shell.openExternal,
  openTab?: (url: string) => boolean,
): void {
  if (contents.session !== session.fromPartition(PREVIEW_PARTITION)) {
    contents.setWindowOpenHandler(({ url }) => {
      if (!isPreviewURL(url) || !openTab?.(url)) {
        void openExternal(url)
      }
      return { action: 'deny' }
    })
    return
  }
  const guard = (event: Event, url: string) => {
    if (!canNavigate(url)) {
      event.preventDefault()
    }
  }
  contents.on('will-navigate', guard)
  contents.on('will-redirect', guard)
  contents.setWindowOpenHandler(({ url, disposition, postBody }) => {
    if (!canNavigate(url)) {
      return { action: 'deny' }
    }
    if (isPreviewURL(url) && !postBody && (disposition === 'foreground-tab' || disposition === 'background-tab')) {
      if (!openTab?.(url)) {
        void openExternal(url)
      }
      return { action: 'deny' }
    }
    return {
      action: 'allow',
      overrideBrowserWindowOptions: {
        frame: true,
        webPreferences: { ...PREVIEW_WEB_PREFERENCES, preload: undefined },
      },
    }
  })
  contents.on('did-create-window', (window, { url }) => {
    registerPreviewPopupOpener(window.webContents, contents)
    const updateTitle = () => {
      const current = window.webContents.getURL() || url
      window.setTitle(current === 'about:blank' ? 'Jaz browser' : new URL(current).origin)
    }
    window.on('page-title-updated', event => event.preventDefault())
    window.webContents.on('did-navigate', updateTitle)
    window.webContents.on('did-navigate-in-page', updateTitle)
    updateTitle()
  })
}
