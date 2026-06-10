import { useSyncExternalStore } from 'react'
import { apiBaseUrl, localBaseUrl, normalizeBaseUrl, setApiBaseUrl } from './api/client'
import { queryClient } from './query/queryClient'

// Gate for the whole app: 'checking' on first probe of the remembered URL,
// then 'connected' while periodic /health polls pass, 'disconnected' once
// they fail — which swaps the app out for the launch screen.
export type ConnectionStatus = 'checking' | 'connected' | 'disconnected'

export type ConnectionState = {
  status: ConnectionStatus
  url: string
  // Set when a connection was lost or a connect attempt failed; the first
  // launch with no backend shows the launch screen without an error.
  error: string | null
}

const POLL_INTERVAL_MS = 5_000
// One missed poll can be a transient blip (remote backends especially);
// only drop to the launch screen after consecutive failures.
const FAILURES_BEFORE_DISCONNECT = 2

let state: ConnectionState = { status: 'checking', url: apiBaseUrl(), error: null }
const listeners = new Set<() => void>()

function setState(next: Partial<ConnectionState>) {
  state = { ...state, ...next }
  for (const l of listeners) l()
}

function subscribe(fn: () => void) {
  listeners.add(fn)
  return () => {
    listeners.delete(fn)
  }
}

export function useConnection(): ConnectionState {
  return useSyncExternalStore(subscribe, () => state)
}

export async function checkHealth(url: string): Promise<boolean> {
  try {
    const res = await fetch(`${url}/health`, { signal: AbortSignal.timeout(3_000) })
    if (!res.ok) return false
    const body = (await res.json()) as { ok?: boolean }
    return body.ok === true
  } catch {
    return false
  }
}

let pollTimer: ReturnType<typeof setTimeout> | null = null
let failures = 0

function stopPolling() {
  if (pollTimer) clearTimeout(pollTimer)
  pollTimer = null
}

function schedulePoll() {
  stopPolling()
  pollTimer = setTimeout(async () => {
    const url = state.url
    const healthy = await checkHealth(url)
    if (state.status !== 'connected' || state.url !== url) return
    if (healthy) {
      failures = 0
    } else {
      failures += 1
      if (failures >= FAILURES_BEFORE_DISCONNECT) {
        setState({ status: 'disconnected', error: `Lost connection to the backend at ${url}` })
        return
      }
    }
    schedulePoll()
  }, POLL_INTERVAL_MS)
}

function markConnected(url: string) {
  const previous = state.url
  setApiBaseUrl(url)
  failures = 0
  // Everything cached belongs to whichever backend answered before; drop it
  // so the app refetches against the one we just connected to.
  if (normalizeBaseUrl(previous) !== normalizeBaseUrl(url)) queryClient.clear()
  setState({ status: 'connected', url: normalizeBaseUrl(url), error: null })
  schedulePoll()
}

// Kept separate from the active URL so the remote option still prefills
// after the user switches back to a local backend.
const REMOTE_URL_KEY = 'jaz.remoteBackendUrl'

export function rememberedRemoteUrl(): string {
  return localStorage.getItem(REMOTE_URL_KEY) ?? ''
}

// Probe whatever URL the user typed; persist it only once it answers.
export async function connectRemote(url: string): Promise<string | null> {
  const target = normalizeBaseUrl(url)
  if (!target) return 'Enter a backend URL'
  if (await checkHealth(target)) {
    localStorage.setItem(REMOTE_URL_KEY, target)
    markConnected(target)
    return null
  }
  return `No backend responded at ${target}`
}

export async function startLocal(): Promise<string | null> {
  if (!window.jaz?.startLocalBackend) {
    return 'Local backend control is only available in the desktop app'
  }
  const result = await window.jaz.startLocalBackend()
  if (!result.ok) return result.error ?? 'Failed to start the backend'
  markConnected(localBaseUrl())
  return null
}

// First probe of the remembered URL, run once at app start.
async function init() {
  const url = state.url
  if (await checkHealth(url)) {
    markConnected(url)
  } else {
    setState({ status: 'disconnected' })
  }
}

void init()
