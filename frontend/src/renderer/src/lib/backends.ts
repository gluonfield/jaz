import { normalizeBaseUrl, setApiAuthToken } from './api/client'

// The backends this device has connected to, so the connect screen can offer
// them as switch targets. Auth tokens already live per-URL under
// `jaz.backendAuth.<url>`, so a remembered backend reconnects with no re-pair.
// "This machine" (loopback) is implicit and never stored here.
const BACKENDS_KEY = 'jaz.backends'

export type KnownBackend = { url: string; label: string; lastConnectedAt: string }

function read(): KnownBackend[] {
  try {
    const parsed = JSON.parse(localStorage.getItem(BACKENDS_KEY) ?? '[]') as KnownBackend[]
    return Array.isArray(parsed) ? parsed.filter((b) => b && typeof b.url === 'string') : []
  } catch {
    return []
  }
}

function write(list: KnownBackend[]): void {
  try {
    localStorage.setItem(BACKENDS_KEY, JSON.stringify(list))
  } catch {
    // a convenience list; never fail a connect over storage
  }
}

// Most-recently-connected first.
export function knownBackends(): KnownBackend[] {
  return read().sort((a, b) => b.lastConnectedAt.localeCompare(a.lastConnectedAt))
}

// Record a successful connection, keyed by normalized origin so reconnecting
// just bumps it. Callers pass only remote backends — local stays implicit.
export function rememberBackend(url: string, now: string): void {
  const target = normalizeBaseUrl(url)
  if (!target) return
  const list = read()
  const previous = list.find((b) => normalizeBaseUrl(b.url) === target)
  const rest = list.filter((b) => normalizeBaseUrl(b.url) !== target)
  write([{ url: target, label: previous?.label || hostLabel(target), lastConnectedAt: now }, ...rest])
}

// Drop a backend from the switcher and forget its key.
export function forgetBackend(url: string): void {
  const target = normalizeBaseUrl(url)
  write(read().filter((b) => normalizeBaseUrl(b.url) !== target))
  setApiAuthToken(target, null)
}

function hostLabel(url: string): string {
  try {
    return new URL(url).host
  } catch {
    return url
  }
}
