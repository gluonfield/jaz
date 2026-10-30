import { app, session, type WebContents, type WebPreferences } from 'electron'
import { BROWSER_PRELOAD_ARGUMENT, PREVIEW_PARTITION, isPreviewURL } from '@shared/preview'

export const PREVIEW_WEB_PREFERENCES: WebPreferences = {
  nodeIntegration: false,
  nodeIntegrationInSubFrames: false,
  nodeIntegrationInWorker: false,
  contextIsolation: true,
  sandbox: true,
  webSecurity: true,
  allowRunningInsecureContent: false,
  webviewTag: false,
  transparent: false,
}

export function attachPreviewWebviews(host: WebContents, preload: string): void {
  host.on('will-attach-webview', (event, preferences, params) => {
    if (!isPreviewURL(params.src) || params.partition !== PREVIEW_PARTITION) {
      event.preventDefault()
      return
    }
    delete (preferences as WebPreferences & { preloadURL?: string }).preloadURL
    preferences.preload = preload
    preferences.additionalArguments = [BROWSER_PRELOAD_ARGUMENT]
    Object.assign(preferences, PREVIEW_WEB_PREFERENCES)
  })
}

export function configureBrowserSessions(chromePreload: string): void {
  const appProduct = `${app.getName().replaceAll(' ', '')}/${app.getVersion()}`
  app.userAgentFallback = app.userAgentFallback
    .replace(` ${appProduct}`, '')
    .replace(` Electron/${process.versions.electron}`, '')
  const browser = session.fromPartition(PREVIEW_PARTITION)
  browser.registerPreloadScript({ type: 'frame', filePath: chromePreload })
  browser.webRequest.onHeadersReceived({ urls: ['https://linkedin.com/*', 'https://www.linkedin.com/*'] }, (details, callback) => {
    const url = new URL(details.url)
    // LinkedIn's guest homepage can be blocked while its sign-in route remains available.
    if (details.resourceType === 'mainFrame' && details.method === 'GET' && details.statusCode === 403 && url.pathname === '/') {
      callback({
        statusLine: 'HTTP/1.1 302 Found',
        responseHeaders: { ...details.responseHeaders, Location: [new URL('/login', url).href] },
      })
    } else {
      callback({})
    }
  })
}
