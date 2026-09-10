import { app, session } from 'electron'
import { PREVIEW_PARTITION } from '@shared/preview'

export function configurePreviewSession(): void {
  const browser = session.fromPartition(PREVIEW_PARTITION)
  const appProduct = `${app.getName().replaceAll(' ', '')}/${app.getVersion()}`
  const userAgent = browser.getUserAgent()
    .replace(` ${appProduct}`, '')
    .replace(` Electron/${process.versions.electron}`, '')
  browser.setUserAgent(userAgent)
}
