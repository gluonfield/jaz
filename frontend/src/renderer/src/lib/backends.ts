import { useSyncExternalStore } from 'react'
import { normalizeBaseUrl, setApiAuthToken } from './api/client'

// The backends this device has connected to, offered as switch targets by the
// connect screen, the sidebar switcher, and Settings. Auth keys already live
// per-URL under `jaz.backendAuth.<url>` and the launch preference under
// `jaz.connection`, so this registry just adds the human-facing name and
// ordering and lives in the same store — one place owns this UI's connection
// memory. "This machine" (loopback) is implicit and never stored here.
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

// Most-recently-connected first.
function sorted(list: KnownBackend[]): KnownBackend[] {
  return [...list].sort((a, b) => b.lastConnectedAt.localeCompare(a.lastConnectedAt))
}

// In-memory mirror so every surface re-renders together on connect/rename/forget
// and useSyncExternalStore gets a stable snapshot between changes.
let cache = sorted(read())
const listeners = new Set<() => void>()

function commit(list: KnownBackend[]): void {
  try {
    localStorage.setItem(BACKENDS_KEY, JSON.stringify(list))
  } catch {
    // a convenience list; never fail a connect over storage
  }
  cache = sorted(list)
  for (const fn of listeners) fn()
}

function subscribe(fn: () => void): () => void {
  listeners.add(fn)
  return () => {
    listeners.delete(fn)
  }
}

export function useKnownBackends(): KnownBackend[] {
  return useSyncExternalStore(subscribe, () => cache)
}

// Record a successful connection, keyed by normalized origin so reconnecting
// just bumps it. Callers pass only remote backends — local stays implicit.
export function rememberBackend(url: string, now: string): void {
  const target = normalizeBaseUrl(url)
  if (!target) return
  const list = read()
  const previous = list.find((b) => normalizeBaseUrl(b.url) === target)
  commit([
    { url: target, label: previous?.label || hostLabel(target), lastConnectedAt: now },
    ...list.filter((b) => normalizeBaseUrl(b.url) !== target),
  ])
}

export function renameBackend(url: string, label: string): void {
  const target = normalizeBaseUrl(url)
  const name = label.trim()
  if (!target || !name) return
  commit(read().map((b) => (normalizeBaseUrl(b.url) === target ? { ...b, label: name } : b)))
}

// Registry-level removal. Prefer connection.forgetBackend, which also clears a
// matching launch preference so a restart doesn't keep aiming at it.
export function removeKnownBackend(url: string): void {
  const target = normalizeBaseUrl(url)
  commit(read().filter((b) => normalizeBaseUrl(b.url) !== target))
  setApiAuthToken(target, null)
}

function hostLabel(url: string): string {
  try {
    return new URL(url).host
  } catch {
    return url
  }
}
