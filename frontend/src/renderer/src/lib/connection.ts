import { useSyncExternalStore } from 'react'
import {
  apiAuthToken,
  apiBaseUrl,
  localBaseUrl,
  normalizeBaseUrl,
  parseBackendConnectUrl,
  setApiAuthToken,
  setApiBaseUrl,
} from './api/client'
import { queryClient } from './query/queryClient'

// Gate for the whole app: 'checking' on first probe of the remembered URL,
// 'connected' while periodic /health polls pass. On loss the app stays
// mounted as 'reconnecting' (banner over live UI, state preserved); only a
// sustained outage degrades to 'disconnected', which swaps in the launch
// screen.
export type ConnectionStatus = 'checking' | 'connected' | 'reconnecting' | 'disconnected'

export type ConnectionState = {
  status: ConnectionStatus
  url: string
  // Set when a connection was lost or a connect attempt failed; the first
  // launch with no backend shows the launch screen without an error.
  error: string | null
}

const POLL_INTERVAL_MS = 5_000
// One missed poll can be a transient blip (remote backends especially);
// only treat the connection as lost after consecutive failures.
const FAILURES_BEFORE_DISCONNECT = 2
// How long 'reconnecting' keeps the app mounted before giving up and
// falling back to the launch screen.
const RECONNECT_GRACE_MS = 30_000

let state: ConnectionState = { status: 'checking', url: apiBaseUrl(), error: null }
const listeners = new Set<() => void>()

type HealthStatus = {
  ok: boolean
  authRequired: boolean
}

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

async function readHealth(url: string): Promise<HealthStatus | null> {
  try {
    const res = await fetch(`${url}/health`, { signal: AbortSignal.timeout(3_000) })
    if (!res.ok) return null
    const body = (await res.json()) as { ok?: boolean; auth_required?: boolean }
    return { ok: body.ok === true, authRequired: body.auth_required === true }
  } catch {
    return null
  }
}

export async function checkHealth(url: string): Promise<boolean> {
  return (await readHealth(url))?.ok === true
}

async function checkAuthAccess(url: string, token: string): Promise<string | null> {
  try {
    const headers = new Headers()
    headers.set('Authorization', `Bearer ${token}`)
    const res = await fetch(`${url}/v1/auth/check`, {
      headers,
      signal: AbortSignal.timeout(3_000),
    })
    if (res.status === 401) return 'Backend key was rejected'
    if (!res.ok) return `Backend auth check failed with ${res.status}`
    return null
  } catch {
    return 'Backend auth check failed'
  }
}

async function verifyBackend(
  url: string,
  token = apiAuthToken(url),
  missingKeyMessage = `Backend at ${url} requires a key`,
): Promise<string | null> {
  const health = await readHealth(url)
  if (!health?.ok) return `No backend responded at ${url}`
  if (!health.authRequired) return null
  if (!token) return missingKeyMessage
  return checkAuthAccess(url, token)
}

let pollTimer: ReturnType<typeof setTimeout> | null = null
let pollGen = 0
let failures = 0
let lostAt = 0

// Polls in every state: while connected it watches for loss; while
// reconnecting/disconnected it keeps probing so the app recovers by itself
// the moment the backend comes back (slow `go run` compile, backend restart,
// login race).
function schedulePoll() {
  const gen = ++pollGen
  if (pollTimer) clearTimeout(pollTimer)
  pollTimer = setTimeout(async () => {
    const url = state.url
    const healthError = await verifyBackend(url)
    if (gen !== pollGen || state.url !== url) return
    if (state.status === 'connected') {
      if (!healthError) {
        failures = 0
      } else {
        failures += 1
        if (failures >= FAILURES_BEFORE_DISCONNECT) {
          lostAt = Date.now()
          setState({ status: 'reconnecting', error: `Lost connection to the backend at ${url}` })
        }
      }
    } else if (!healthError) {
      markConnected(url)
      return
    } else if (state.status === 'reconnecting' && Date.now() - lostAt > RECONNECT_GRACE_MS) {
      setState({ status: 'disconnected', error: healthError })
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
  const parsed = parseBackendConnectUrl(url)
  const target = normalizeBaseUrl(parsed.url)
  if (!target) return 'Enter a backend URL'
  const token = parsed.key || apiAuthToken(target)
  const error = await verifyBackend(target, token, 'Paste the client URL from the backend; it includes the key')
  if (error) return error
  if (parsed.key) setApiAuthToken(target, parsed.key)
  localStorage.setItem(REMOTE_URL_KEY, target)
  markConnected(target)
  return null
}

export async function startLocal(): Promise<string | null> {
  if (!window.jaz?.startLocalBackend) {
    return 'Local backend control is only available in the desktop app'
  }
  const result = await window.jaz.startLocalBackend()
  if (!result.ok) return result.error ?? 'Failed to start the backend'
  // connect to the URL the main process actually verified, not the env default
  const url = result.url ?? localBaseUrl()
  if (result.key) setApiAuthToken(url, result.key)
  markConnected(url)
  return null
}

// First probe of the remembered URL, run once at app start.
async function init() {
  const url = state.url
  const error = await verifyBackend(url)
  if (!error) {
    markConnected(url)
  } else {
    setState({ status: 'disconnected', error })
    schedulePoll()
  }
}

void init()
