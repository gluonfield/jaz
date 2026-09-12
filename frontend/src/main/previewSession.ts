import { app, session, type WebContents, type WebPreferences } from 'electron'
import { BROWSER_PRELOAD_ARGUMENT, PREVIEW_PARTITION, isPreviewURL } from '@shared/preview'

export function attachPreviewWebviews(host: WebContents, preload: string): void {
  host.on('will-attach-webview', (event, preferences, params) => {
    if (!isPreviewURL(params.src) || params.partition !== PREVIEW_PARTITION) {
      event.preventDefault()
      return
    }
    delete (preferences as WebPreferences & { preloadURL?: string }).preloadURL
    preferences.preload = preload
    preferences.additionalArguments = [BROWSER_PRELOAD_ARGUMENT]
    preferences.nodeIntegration = false
    preferences.nodeIntegrationInSubFrames = false
    preferences.contextIsolation = true
    preferences.sandbox = true
    preferences.webSecurity = true
    preferences.allowRunningInsecureContent = false
  })
}

export function configurePreviewSession(): void {
  const browser = session.fromPartition(PREVIEW_PARTITION)
  const appProduct = `${app.getName().replaceAll(' ', '')}/${app.getVersion()}`
  const userAgent = browser.getUserAgent()
    .replace(` ${appProduct}`, '')
    .replace(` Electron/${process.versions.electron}`, '')
  browser.setUserAgent(userAgent)
}
