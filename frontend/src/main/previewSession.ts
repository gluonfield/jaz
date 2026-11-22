import { app, ipcMain, session, type WebContents, type WebPreferences } from 'electron'
import {
  BROWSER_PASSKEY_FALLBACK_ARGUMENT,
  BROWSER_PASSKEY_FALLBACK_CHANNEL,
  BROWSER_PRELOAD_ARGUMENT,
  PREVIEW_PARTITION,
  isPreviewURL,
} from '@shared/preview'
import { isTrustedRendererURL } from '@main/permissions'

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

let browserPasskeyFallback = false
let passkeyFallbackListening = false
const popupOpeners = new WeakMap<WebContents, WebContents>()

export function registerPreviewPopupOpener(popup: WebContents, opener: WebContents): void {
  popupOpeners.set(popup, opener)
}

function previewWebviewOwner(contents: WebContents): WebContents | null {
  const seen = new Set<WebContents>()
  let current: WebContents | undefined = contents
  while (current && !current.isDestroyed() && !seen.has(current)) {
    if (current.getType() === 'webview') {
      return current
    }
    seen.add(current)
    current = popupOpeners.get(current)
  }
  return null
}

export function previewHostPreferences(preload: string): WebPreferences {
  return {
    preload,
    contextIsolation: true,
    nodeIntegration: false,
    sandbox: false,
    webviewTag: true,
    additionalArguments: browserPasskeyFallback ? [BROWSER_PASSKEY_FALLBACK_ARGUMENT] : [],
  }
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

export function configureBrowserSessions(chromePreload: string, passkeyFallbackPreload: string): void {
  const browserApp = app as typeof app & { enableWebAuthnBrowserAuthenticator?: () => boolean }
  browserPasskeyFallback = process.platform === 'darwin' && browserApp.enableWebAuthnBrowserAuthenticator?.() !== true
  const appProduct = `${app.getName().replaceAll(' ', '')}/${app.getVersion()}`
  app.userAgentFallback = app.userAgentFallback
    .replace(` ${appProduct}`, '')
    .replace(` Electron/${process.versions.electron}`, '')
  const browser = session.fromPartition(PREVIEW_PARTITION)
  browser.registerPreloadScript({ type: 'frame', filePath: chromePreload })
  if (browserPasskeyFallback) {
    browser.registerPreloadScript({ type: 'frame', filePath: passkeyFallbackPreload })
    if (!passkeyFallbackListening) {
      passkeyFallbackListening = true
      ipcMain.on(BROWSER_PASSKEY_FALLBACK_CHANNEL, (event) => {
        const sender = event.sender
        if (sender.isDestroyed() || sender.session !== browser) {
          return
        }
        const owner = previewWebviewOwner(sender)
        const host = owner?.hostWebContents
        if (owner && owner.session === browser && host && !host.isDestroyed() && isTrustedRendererURL(host.getURL())) {
          host.send(BROWSER_PASSKEY_FALLBACK_CHANNEL, owner.id)
        }
      })
    }
  }
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
