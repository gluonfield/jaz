import type { IpcMainInvokeEvent, WebContents } from 'electron'

interface PermissionDetails {
  isMainFrame: boolean
  requestingUrl?: string
}

export function isTrustedRendererURL(value: string | undefined): boolean {
  if (!value) return false
  try {
    const url = new URL(value)
    const devURL = process.env['ELECTRON_RENDERER_URL']
    if (devURL) return url.origin === new URL(devURL).origin
    return url.protocol === 'file:' && url.pathname.endsWith('/renderer/index.html')
  } catch {
    return false
  }
}

export function canGrantAppPermission(
  contents: WebContents | null,
  permission: string,
  details: PermissionDetails,
): boolean {
  if (permission !== 'media' && permission !== 'local-fonts') return false
  if (!contents || contents.isDestroyed() || contents.getType() !== 'window') return false
  if (!details.isMainFrame) return false
  return isTrustedRendererURL(details.requestingUrl || contents.getURL())
}

// IPC that drives the machine (computer use, system permission prompts) only
// answers Jaz's own top-level renderer; returns the sender's id.
export function trustedWindow(event: IpcMainInvokeEvent): number {
  if (event.sender.getType() !== 'window' || event.senderFrame !== event.sender.mainFrame || !isTrustedRendererURL(event.senderFrame.url)) {
    throw new Error('Requires a trusted Jaz window')
  }
  return event.sender.id
}
