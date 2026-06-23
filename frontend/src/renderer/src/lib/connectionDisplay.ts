import { normalizeBaseUrl } from './api/client'
import { knownBackendLabel } from './backends'
import { isLoopbackUrl, type ConnectionStatus } from './connection'
import { localDeviceLabel } from './deviceLabel'

export type BackendDescription = {
  // A loopback backend is "this machine"; everything else is a server.
  local: boolean
  // "This Mac" or the server's host — what to show at a glance.
  title: string
  url: string
}

// The local machine's name in switch lists ("This Mac"), capitalized for
// standalone use unlike the lowercase "Run on this Mac" CTA.
export function localBackendLabel(): string {
  return capitalize(localDeviceLabel())
}

// Whether two URLs point at the same backend — the canonical "is this the one
// we're on?" check, so every switch list compares identity the same way.
export function sameBackend(a: string, b: string): boolean {
  return normalizeBaseUrl(a) === normalizeBaseUrl(b)
}

// The name to show for a backend: "This Mac" for local, the user-given name for
// a known remote, else its host. Prefer this over describeBackend().title
// wherever the current backend is named, so a renamed server reads as its name
// (not its IP) — e.g. the sidebar indicator and the switch animation.
export function backendName(url: string): string {
  const backend = describeBackend(url)
  return backend.local ? backend.title : knownBackendLabel(url) || backend.title
}

// Names whichever backend a URL points at so the sidebar and the switcher can
// show it the same way.
export function describeBackend(url: string): BackendDescription {
  if (!url || isLoopbackUrl(url)) {
    return { local: true, title: capitalize(localDeviceLabel()), url }
  }
  try {
    return { local: false, title: new URL(url).host, url }
  } catch {
    return { local: false, title: url, url }
  }
}

// The status dot/label shared by the footer indicator and the switcher's
// current-connection card. Only 'connected'/'reconnecting' show while the app
// is mounted, but the switcher can briefly observe the others mid-switch.
export function connectionStatusDisplay(status: ConnectionStatus): { dot: string; label: string } {
  switch (status) {
    case 'connected':
      return { dot: 'bg-ok', label: 'Connected' }
    case 'reconnecting':
      return { dot: 'bg-running animate-pulse', label: 'Reconnecting…' }
    case 'checking':
      return { dot: 'bg-running animate-pulse', label: 'Connecting…' }
    case 'pending_approval':
      return { dot: 'bg-running animate-pulse', label: 'Waiting for approval' }
    case 'disconnected':
      return { dot: 'bg-danger', label: 'Disconnected' }
  }
}

function capitalize(value: string): string {
  return value ? value.charAt(0).toUpperCase() + value.slice(1) : value
}
