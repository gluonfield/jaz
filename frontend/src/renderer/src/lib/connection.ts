import { useSyncExternalStore } from 'react'
import {
  apiAuthToken,
  apiBaseUrl,
  CLIENT_PLATFORM,
  CLIENT_PLATFORM_HEADER,
  localBaseUrl,
  normalizeBaseUrl,
  parseBackendConnectUrl,
  setApiAuthToken,
  setApiBaseUrl,
} from './api/client'
import { rememberBackend, removeKnownBackend } from './backends'
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
    headers.set(CLIENT_PLATFORM_HEADER, CLIENT_PLATFORM)
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
// What we were connected to before opening a device approval, so abandoning it
// returns there. Only set when the approval was started from a live connection
// (a mid-session machine switch); null on first-run/reconnect.
type ConnectionSnapshot = { url: string; preference: ConnectionPreference | null }
let preflightSnapshot: ConnectionSnapshot | null = null
// The backend the React Query cache currently holds data for. Tracked apart
// from state.url because markPendingApproval parks the pending URL in state.url
// before approval, which would otherwise hide a real backend switch from the
// cache-clear check in markConnected.
let cacheOwnerUrl = ''

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
  preflightSnapshot = null
  const normalized = normalizeBaseUrl(url)
  setApiBaseUrl(url)
  failures = 0
  // Every successful connection is a switch target next time; local is implicit.
  if (!isLoopbackUrl(url)) rememberBackend(url, new Date().toISOString())
  // The cache belongs to the backend we were last connected to; drop it when we
  // actually connect somewhere new so the app refetches against the right one.
  if (cacheOwnerUrl !== normalized) queryClient.clear()
  cacheOwnerUrl = normalized
  setState({ status: 'connected', url: normalized, pairing: null, error: null })
  schedulePoll()
}

// Re-probe a backend whose identity we already trust — a saved remote at
// startup, or the one we were on before a cancelled approval — and either
// reconnect or surface why it's now unreachable.
async function reconnectKnown(url: string): Promise<void> {
  setState({ status: 'checking', url, pairing: null, error: null })
  const error = await verifyBackend(url)
  if (error) {
    setState({ status: 'disconnected', url, pairing: null, error })
    schedulePoll()
    return
  }
  markConnected(url)
}

async function registerDevice(url: string, rootToken: string): Promise<string | null> {
  try {
    const profile = await getDeviceProfile()
    const res = await fetch(`${url}/v1/devices/register`, {
      method: 'POST',
      headers: {
        Authorization: `Bearer ${rootToken}`,
        'Content-Type': 'application/json',
        [CLIENT_PLATFORM_HEADER]: CLIENT_PLATFORM,
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
      markConnected(url)
      rememberLocalLaunch(url)
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
      rememberLocalLaunch(url)
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
      headers: { 'Content-Type': 'application/json', [CLIENT_PLATFORM_HEADER]: CLIENT_PLATFORM },
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
    rememberLocalLaunch(url)
    return null
  } catch {
    return 'Could not create a device approval request'
  }
}

function markPendingApproval(pairing: PendingPairing) {
  if (pollTimer) clearTimeout(pollTimer)
  // Snapshot a healthy connection before we re-point at the pending backend,
  // so cancel/expire/reject can return to it.
  preflightSnapshot =
    state.status === 'connected' ? { url: state.url, preference: connectionPreference() } : null
  setApiBaseUrl(pairing.url)
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
        headers: { [CLIENT_PLATFORM_HEADER]: CLIENT_PLATFORM },
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
        abandonPairing('Device approval request expired')
        return
      }
      if (body?.pairing?.status === 'rejected') {
        abandonPairing('Device approval was rejected')
        return
      }
    } catch {
      // Keep waiting; the backend may be restarting while approval is open.
    }
    schedulePairingPoll()
  }, 2_000)
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
    // A remote preference is only usable with its URL; a malformed one falls
    // through to null so init takes the first-launch (local) path instead of
    // reconnecting to whatever stale base URL it would otherwise borrow.
    if (parsed.mode === 'remote' && parsed.remoteUrl) return { mode: 'remote', remoteUrl: parsed.remoteUrl }
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

// Persist the launch default eagerly only for a local backend, which is always
// usable. A remote is deferred until its onboarding is confirmed complete (see
// persistLaunchPreference), so bailing out of an unfinished remote never leaves
// a restart auto-connecting straight back into a setup it can't finish.
function rememberLocalLaunch(url: string): void {
  if (isLoopbackUrl(url)) savePreference({ mode: 'local' })
}

// Commit a backend as the one to reach on launch. Called once onboarding for it
// is confirmed complete, so only a usable backend becomes the boot default —
// including remotes, which rememberLocalLaunch deliberately skips.
export function persistLaunchPreference(url: string): void {
  savePreference(isLoopbackUrl(url) ? { mode: 'local' } : { mode: 'remote', remoteUrl: normalizeBaseUrl(url) })
}

// Forget a saved backend everywhere: drop it from the registry and its key, and
// clear the launch preference if it pointed here, so a restart never keeps
// aiming at a backend the user just removed.
export function forgetBackend(url: string): void {
  removeKnownBackend(url)
  const pref = connectionPreference()
  if (pref?.mode === 'remote' && pref.remoteUrl && normalizeBaseUrl(pref.remoteUrl) === normalizeBaseUrl(url)) {
    clearConnectionPreference()
  }
}

// Leave the current backend for the connect chooser without auto-reconnecting.
// The escape from a backend whose onboarding you can't or won't finish: it stops
// the health poll so it can't flip back to connected, and leaves the launch
// preference pointing at the previously set-up backend (or nothing).
export function disconnectBackend(): void {
  if (pollTimer) clearTimeout(pollTimer)
  if (pairingTimer) clearTimeout(pairingTimer)
  pollGen += 1
  pairingGen += 1
  preflightSnapshot = null
  setState({ status: 'disconnected', pairing: null, error: null })
}

export function cancelPendingApproval(): void {
  abandonPairing(null)
}

// Single exit for a pending approval that won't complete. If it was opened over
// a working connection, return there; otherwise fall back to the launch screen
// with the reason, if any.
function abandonPairing(error: string | null): void {
  if (pairingTimer) clearTimeout(pairingTimer)
  pairingGen += 1
  const snapshot = preflightSnapshot
  preflightSnapshot = null
  if (snapshot) {
    void restoreConnection(snapshot)
    return
  }
  setState({ status: 'disconnected', pairing: null, error })
}

async function restoreConnection(snapshot: ConnectionSnapshot): Promise<void> {
  // Restore the launch preference first so a failed re-probe degrades exactly
  // like any other loss of that backend.
  if (snapshot.preference) savePreference(snapshot.preference)
  await reconnectKnown(snapshot.url)
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
  markConnected(target)
  // A remote becomes the launch default only once its onboarding completes, so
  // a restart never auto-reconnects into a setup the user bailed on.
  return null
}

export async function startLocal(): Promise<string | null> {
  if (!window.jaz?.startLocalBackend) {
    return 'Local backend control is only available in the desktop app'
  }
  const result = await window.jaz.startLocalBackend()
  const url = normalizeBaseUrl(result.url ?? localBaseUrl())
  if (await connectStoredToken(url)) {
    savePreference({ mode: 'local' })
    return null
  }
  if (!result.ok) return result.error ?? 'Failed to start the backend'
  if (result.key) {
    const error = await registerDevice(url, result.key)
    if (state.status === 'pending_approval' || error) return error
    return null
  }
  const error = await verifyBackend(
    url,
    '',
    `Backend at ${url} requires a key. Paste its client URL or stop that backend and start locally.`,
  )
  if (error) return error
  markConnected(url)
  savePreference({ mode: 'local' })
  return null
}

async function connectStoredToken(url: string): Promise<boolean> {
  const token = apiAuthToken(url)
  if (!token) return false
  if (!(await verifyBackend(url, token))) {
    markConnected(url)
    return true
  }
  return false
}

// Run once at app start. The branch hinges on the saved preference so first
// launch never shows a connection error: only an *expected* backend (a saved
// remote, or a local start the user already opted into) can surface one.
async function init() {
  const pref = connectionPreference()

  // A remote server was expected here, so reconnectKnown surfacing the failure
  // (rather than a silent welcome) is right.
  if (pref?.mode === 'remote') {
    await reconnectKnown(normalizeBaseUrl(pref.remoteUrl || state.url))
    return
  }

  // Both remaining paths target the local backend. Aim the store — and the
  // recovery poll they fall back to — at local, never the stale persisted base
  // URL, which could be a remote the user connected to but bailed on.
  const localUrl = normalizeBaseUrl(localBaseUrl())

  if (pref?.mode === 'local') {
    setState({ status: 'checking', url: localUrl, error: null })
    // startLocal() flips the store to connected on success; only a genuine
    // failure (or a non-desktop runtime) falls through to the choice screen.
    const error = await startLocal()
    if (error) {
      setState({ status: 'disconnected', error })
      schedulePoll()
    }
    return
  }

  // First launch, no saved preference. Adopt local if it already answers,
  // otherwise present the welcome choice with NO error — nothing to disappoint
  // yet.
  const error = await verifyBackend(localUrl)
  if (!error) {
    savePreference({ mode: 'local' })
    markConnected(localUrl)
    return
  }
  setState({ status: 'disconnected', url: localUrl, error: null })
  schedulePoll()
}

void init()
