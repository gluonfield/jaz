import { resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import type { IpcMainEvent, IpcMainInvokeEvent, Session, WebContents } from 'electron'

interface PermissionDetails {
  isMainFrame: boolean
  requestingUrl?: string
}

let trustedRendererPath = resolve(__dirname, '../renderer/index.html')

export function setTrustedRendererPath(path: string): void {
  trustedRendererPath = resolve(path)
}

export function getTrustedRendererPath(): string {
  return trustedRendererPath
}

export function isTrustedRendererURL(value: string | undefined, expectedPath = trustedRendererPath): boolean {
  if (!value) return false
  try {
    const url = new URL(value)
    const devURL = process.env['ELECTRON_RENDERER_URL']
    if (devURL) return url.origin === new URL(devURL).origin
    return url.protocol === 'file:' && resolve(fileURLToPath(url)) === resolve(expectedPath)
  } catch {
    return false
  }
}

export function canGrantAppPermission(
  contents: WebContents | null,
  permission: string,
  details: PermissionDetails,
): boolean {
  if (!contents || contents.isDestroyed() || contents.getType() !== 'window') return false
  if (permission === 'clipboard-sanitized-write') {
    return isTrustedRendererURL(contents.getURL())
  }
  if (permission !== 'media' && permission !== 'local-fonts') return false
  if (!details.isMainFrame) return false
  return isTrustedRendererURL(details.requestingUrl || contents.getURL())
}

export function configureAppPermissions(target: Session): void {
  target.setPermissionRequestHandler((contents, permission, callback, details) => {
    callback(canGrantAppPermission(contents, permission, details))
  })
  target.setPermissionCheckHandler((contents, permission, _origin, details) =>
    canGrantAppPermission(contents, permission, details),
  )
}

// IPC that drives the machine (computer use, system permission prompts) only
// answers Jaz's own top-level renderer; returns the sender's id.
export function trustedWindow(event: IpcMainEvent | IpcMainInvokeEvent): number {
  if (event.sender.getType() !== 'window' || event.senderFrame !== event.sender.mainFrame || !isTrustedRendererURL(event.senderFrame.url)) {
    throw new Error('Requires a trusted Jaz window')
  }
  return event.sender.id
}
