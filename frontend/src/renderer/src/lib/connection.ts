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
import { getDeviceProfile } from './deviceIdentity'
import { queryClient } from './query/queryClient'

// Gate for the whole app: 'checking' on first probe of the remembered URL,
// 'connected' while periodic /health polls pass. On loss the app stays
// mounted as 'reconnecting' (banner over live UI, state preserved); only a
// sustained outage degrades to 'disconnected', which swaps in the launch
// screen.
export type ConnectionStatus = 'checking' | 'connected' | 'reconnecting' | 'disconnected' | 'pending_approval'

export type PendingPairing = {
  url: string
  id: string
  secret: string
  deviceName: string
  expiresAt: string
}

export type ConnectionState = {
  status: ConnectionStatus
  url: string
  pairing: PendingPairing | null
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

let state: ConnectionState = { status: 'checking', url: apiBaseUrl(), pairing: null, error: null }
const listeners = new Set<() => void>()

type HealthStatus = {
  ok: boolean
  authRequired: boolean
}

type AuthAccess = {
  ok: boolean
  error?: string
  code?: string
  authKind?: string
  deviceId?: string
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

async function readJSON<T>(res: Response): Promise<T | null> {
  try {
    return (await res.json()) as T
  } catch {
    return null
  }
}

export async function checkHealth(url: string): Promise<boolean> {
  return (await readHealth(url))?.ok === true
}

async function checkAuthAccess(url: string, token: string): Promise<AuthAccess> {
  try {
    const headers = new Headers()
    headers.set('Authorization', `Bearer ${token}`)
    const res = await fetch(`${url}/v1/auth/check`, {
      headers,
      signal: AbortSignal.timeout(3_000),
    })
    const body = await readJSON<{ error?: string; code?: string; auth_kind?: string; device_id?: string }>(res)
    if (res.status === 401) return { ok: false, error: 'Backend key was rejected', code: body?.code }
    if (!res.ok) {
      return {
        ok: false,
        error: body?.error || `Backend auth check failed with ${res.status}`,
        code: body?.code,
      }
    }
    return { ok: true, authKind: body?.auth_kind, deviceId: body?.device_id }
  } catch {
    return { ok: false, error: 'Backend auth check failed' }
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
  const access = await checkAuthAccess(url, token)
  return access.ok ? null : access.error || 'Backend auth check failed'
}

let pollTimer: ReturnType<typeof setTimeout> | null = null
let pairingTimer: ReturnType<typeof setTimeout> | null = null
let pollGen = 0
let pairingGen = 0
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
  if (pairingTimer) clearTimeout(pairingTimer)
  pairingGen += 1
  const previous = state.url
  setApiBaseUrl(url)
  failures = 0
  // Everything cached belongs to whichever backend answered before; drop it
  // so the app refetches against the one we just connected to.
  if (normalizeBaseUrl(previous) !== normalizeBaseUrl(url)) queryClient.clear()
  setState({ status: 'connected', url: normalizeBaseUrl(url), pairing: null, error: null })
  schedulePoll()
}

async function registerDevice(url: string, rootToken: string): Promise<string | null> {
  try {
    const profile = await getDeviceProfile()
    const res = await fetch(`${url}/v1/devices/register`, {
      method: 'POST',
      headers: {
        Authorization: `Bearer ${rootToken}`,
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({ ...profile, kind: 'desktop' }),
      signal: AbortSignal.timeout(5_000),
    })
    const body = await readJSON<{
      token?: string
      pairing?: { id: string; expires_at: string; device?: { name?: string } }
      pairing_secret?: string
      error?: string
    }>(res)
    if (res.ok && body?.token) {
      setApiAuthToken(url, body.token)
      localStorage.setItem(REMOTE_URL_KEY, normalizeBaseUrl(url))
      markConnected(url)
      savePreference(isLoopbackUrl(url) ? { mode: 'local' } : { mode: 'remote', remoteUrl: normalizeBaseUrl(url) })
      return null
    }
    if (res.status === 202 && body?.pairing && body.pairing_secret) {
      markPendingApproval({
        url,
        id: body.pairing.id,
        secret: body.pairing_secret,
        deviceName: body.pairing.device?.name || profile.name,
        expiresAt: body.pairing.expires_at,
      })
      savePreference(isLoopbackUrl(url) ? { mode: 'local' } : { mode: 'remote', remoteUrl: normalizeBaseUrl(url) })
      return null
    }
    return body?.error || `Device registration failed with ${res.status}`
  } catch {
    return 'Device registration failed'
  }
}

async function startPairing(url: string): Promise<string | null> {
  try {
    const profile = await getDeviceProfile()
    const res = await fetch(`${url}/v1/devices/pairing-requests`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ ...profile, kind: 'desktop' }),
      signal: AbortSignal.timeout(5_000),
    })
    const body = await readJSON<{
      pairing?: { id: string; expires_at: string; device?: { name?: string } }
      pairing_secret?: string
      error?: string
    }>(res)
    if (!res.ok || !body?.pairing || !body.pairing_secret) {
      return body?.error || 'Paste the client URL from the backend; it includes the key'
    }
    markPendingApproval({
      url,
      id: body.pairing.id,
      secret: body.pairing_secret,
      deviceName: body.pairing.device?.name || profile.name,
      expiresAt: body.pairing.expires_at,
    })
    savePreference(isLoopbackUrl(url) ? { mode: 'local' } : { mode: 'remote', remoteUrl: normalizeBaseUrl(url) })
    return null
  } catch {
    return 'Could not create a device approval request'
  }
}

function markPendingApproval(pairing: PendingPairing) {
  if (pollTimer) clearTimeout(pollTimer)
  setApiBaseUrl(pairing.url)
  localStorage.setItem(REMOTE_URL_KEY, normalizeBaseUrl(pairing.url))
  setState({ status: 'pending_approval', url: normalizeBaseUrl(pairing.url), pairing, error: null })
  schedulePairingPoll()
}

function schedulePairingPoll() {
  const pairing = state.pairing
  if (!pairing || state.status !== 'pending_approval') return
  const gen = ++pairingGen
  if (pairingTimer) clearTimeout(pairingTimer)
  pairingTimer = setTimeout(async () => {
    if (gen !== pairingGen || state.status !== 'pending_approval') return
    try {
      const params = new URLSearchParams({ secret: pairing.secret })
      const res = await fetch(`${pairing.url}/v1/devices/pairing-requests/${encodeURIComponent(pairing.id)}?${params}`, {
        signal: AbortSignal.timeout(5_000),
      })
      const body = await readJSON<{
        token?: string
        pairing?: { status: string }
        error?: string
      }>(res)
      if (res.ok && body?.pairing?.status === 'approved') {
        setApiAuthToken(pairing.url, body.token || pairing.secret)
        markConnected(pairing.url)
        return
      }
      if (res.status === 410 || body?.pairing?.status === 'expired') {
        setState({ status: 'disconnected', pairing: null, error: 'Device approval request expired' })
        return
      }
      if (body?.pairing?.status === 'rejected') {
        setState({ status: 'disconnected', pairing: null, error: 'Device approval was rejected' })
        return
      }
    } catch {
      // Keep waiting; the backend may be restarting while approval is open.
    }
    schedulePairingPoll()
  }, 2_000)
}

// Kept separate from the active URL so the remote option still prefills
// after the user switches back to a local backend.
const REMOTE_URL_KEY = 'jaz.remoteBackendUrl'

export function rememberedRemoteUrl(): string {
  return localStorage.getItem(REMOTE_URL_KEY) ?? ''
}

// How the app should reach a backend on launch. 'local' = start (or adopt) a
// backend on this machine every time; 'remote' = connect to a saved server.
// Persisting the choice is what lets a first launch present a clean welcome
// instead of a connection error, and lets later launches act without asking.
export type ConnectionMode = 'local' | 'remote'
type ConnectionPreference = { mode: ConnectionMode; remoteUrl?: string }

const PREFERENCE_KEY = 'jaz.connection'

export function connectionPreference(): ConnectionPreference | null {
  try {
    const raw = localStorage.getItem(PREFERENCE_KEY)
    if (!raw) return null
    const parsed = JSON.parse(raw) as Partial<ConnectionPreference>
    if (parsed.mode === 'local') return { mode: 'local' }
    if (parsed.mode === 'remote') return { mode: 'remote', remoteUrl: parsed.remoteUrl }
    return null
  } catch {
    return null
  }
}

function savePreference(pref: ConnectionPreference): void {
  try {
    localStorage.setItem(PREFERENCE_KEY, JSON.stringify(pref))
  } catch {
    // preference is a convenience; never fail a connect over storage
  }
}

export function clearConnectionPreference(): void {
  localStorage.removeItem(PREFERENCE_KEY)
}

export function cancelPendingApproval(): void {
  if (pairingTimer) clearTimeout(pairingTimer)
  pairingGen += 1
  setState({ status: 'disconnected', pairing: null, error: null })
}

// A loopback host is "this machine" — treat localhost and 127.0.0.1 alike since
// the spawned backend reports 127.0.0.1 while the env default is localhost.
export function isLoopbackUrl(url: string): boolean {
  try {
    const host = new URL(url).hostname
    return host === 'localhost' || host === '127.0.0.1' || host === '::1' || host === '0.0.0.0'
  } catch {
    return false
  }
}

// Probe whatever URL the user typed; persist it only once it answers.
export async function connectRemote(url: string): Promise<string | null> {
  const parsed = parseBackendConnectUrl(url)
  const target = normalizeBaseUrl(parsed.url)
  if (!target) return 'Enter a backend URL'
  const token = parsed.key || apiAuthToken(target)
  const health = await readHealth(target)
  if (!health?.ok) return `No backend responded at ${target}`
  if (health.authRequired) {
    if (!token) return startPairing(target)
    const access = await checkAuthAccess(target, token)
    if (access.ok) {
      if (access.authKind === 'root') {
        return registerDevice(target, token)
      }
      if (parsed.key) setApiAuthToken(target, parsed.key)
    } else if (access.code === 'device_approval_required') {
      return registerDevice(target, token)
    } else {
      return access.error ?? 'Backend auth check failed'
    }
  }
  localStorage.setItem(REMOTE_URL_KEY, target)
  markConnected(target)
  savePreference({ mode: 'remote', remoteUrl: target })
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
  if (result.key) {
    const error = await registerDevice(url, result.key)
    if (state.status === 'pending_approval' || error) return error
  }
  markConnected(url)
  savePreference({ mode: 'local' })
  return null
}

// Run once at app start. The branch hinges on the saved preference so first
// launch never shows a connection error: only an *expected* backend (a saved
// remote, or a local start the user already opted into) can surface one.
async function init() {
  const pref = connectionPreference()

  if (pref?.mode === 'remote') {
    const url = normalizeBaseUrl(pref.remoteUrl || state.url)
    setState({ status: 'checking', url, error: null })
    const error = await verifyBackend(url)
    if (!error) {
      markConnected(url)
      return
    }
    // A remote server was expected here, so naming the failure is right.
    setState({ status: 'disconnected', url, error })
    schedulePoll()
    return
  }

  if (pref?.mode === 'local') {
    setState({ status: 'checking', error: null })
    // startLocal() flips the store to connected on success; only a genuine
    // failure (or a non-desktop runtime) falls through to the choice screen.
    const error = await startLocal()
    if (error) {
      setState({ status: 'disconnected', error })
      schedulePoll()
    }
    return
  }

  // First launch, no saved preference. Adopt a backend if one already answers,
  // but otherwise present the welcome choice with NO error — there is no
  // expectation to disappoint yet.
  const url = state.url
  const error = await verifyBackend(url)
  if (!error) {
    savePreference({ mode: 'local' })
    markConnected(url)
    return
  }
  setState({ status: 'disconnected', error: null })
  schedulePoll()
}

void init()
