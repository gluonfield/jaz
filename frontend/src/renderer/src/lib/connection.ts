import { useEffect, useRef, useSyncExternalStore } from 'react'
import {
  apiBaseUrl, cachedApiAuthToken, clearApiSession, CLIENT_PLATFORM, CLIENT_PLATFORM_HEADER,
  consumeStartupConnectUrl, isLocalBackendUrl, localBaseUrl, normalizeBaseUrl,
  parseBackendConnectUrl, rememberApiAuthToken, restoreApiAuthToken, secureBackendConnection,
  setApiAuthToken, setApiBaseUrl, onApiAuthRejected,
} from '@/lib/api/client'
import { rememberBackend, removeKnownBackend } from '@/lib/backends'
import { clientRuntime } from '@/lib/clientRuntime'
import { getDeviceProfile } from '@/lib/deviceIdentity'
import { queryClient } from '@/lib/query/queryClient'
import { readAccess, readHealth, readJSON } from '@/lib/connectionProbe'
import type { ResourceTicket, SessionSnapshot, VerifiedSession } from '@shared/connectionSession'

export { isLocalBackendUrl } from '@/lib/api/client'
export type ConnectionStatus = 'checking' | 'connected' | 'reconnecting' | 'disconnected' | 'pending_approval'
export type PendingPairing = { url: string; id: string; secret: string; deviceName: string; expiresAt: string }
export type ConnectionState = { status: ConnectionStatus; url: string; pairing: PendingPairing | null; error: string | null }
export type ConnectionMode = 'local' | 'remote'
type ConnectionPreference = { mode: ConnectionMode; remoteUrl?: string }
type Attempt = { url: string; revision: number; controller: AbortController }
type Candidate = VerifiedSession & { ticket?: ResourceTicket }
const LOCAL_URL = 'http://127.0.0.1:5299'
const PREFERENCE_KEY = 'jaz.connection'
const bridge = ['main', 'board', 'launcher'].includes(clientRuntime.windowKind) ? clientRuntime.connectionSession : undefined
const desktop = bridge?.read()
export const ownsConnection = desktop?.owner ?? clientRuntime.windowKind === 'main'
let authoritative = desktop?.snapshot
let attempt: Attempt | null = null
let pollTimer: ReturnType<typeof setTimeout> | undefined
let pairingTimer: ReturnType<typeof setTimeout> | undefined
let issuance: Promise<unknown> = Promise.resolve()
let state: ConnectionState = { status: 'checking', url: apiBaseUrl(), pairing: null, error: null }
const listeners = new Set<() => void>()

function setState(next: Partial<ConnectionState>): void {
  state = { ...state, ...next }
  for (const listener of listeners) {
    listener()
  }
}

export function useConnection(): ConnectionState {
  return useSyncExternalStore((listener) => {
    listeners.add(listener)
    return () => listeners.delete(listener)
  }, () => state)
}

export function useBackendChange(onChange: (url: string) => void): void {
  const { url, status } = useConnection()
  const previous = useRef<string | null>(null)
  const handler = useRef(onChange)
  handler.current = onChange
  useEffect(() => {
    if (status !== 'connected' || previous.current === url) {
      return
    }
    const changed = previous.current !== null
    previous.current = url
    if (changed) {
      handler.current(url)
    }
  }, [url, status])
}

export function connectionPreference(): ConnectionPreference | null {
  try {
    const value = JSON.parse(localStorage.getItem(PREFERENCE_KEY) || 'null') as ConnectionPreference | null
    if (value?.mode === 'local') {
      return { mode: 'local' }
    }
    if (value?.mode === 'remote' && value.remoteUrl) {
      return { mode: 'remote', remoteUrl: normalizeBaseUrl(value.remoteUrl) }
    }
  } catch {
    return null
  }
  return null
}

function savePreference(value: ConnectionPreference): void {
  try {
    localStorage.setItem(PREFERENCE_KEY, JSON.stringify(value))
  } catch {
    // Launch intent is independent of current authentication.
  }
}

export function clearConnectionPreference(): void {
  localStorage.removeItem(PREFERENCE_KEY)
}

function invalidate(): void {
  attempt?.controller.abort()
  attempt = null
  clearTimeout(pollTimer)
  clearTimeout(pairingTimer)
  clearApiSession()
  void queryClient.cancelQueries()
  queryClient.clear()
}

function current(value: Attempt): boolean {
  return attempt === value && !value.controller.signal.aborted && (!authoritative || authoritative.revision === value.revision)
}

function begin(url: string, announce = true): Attempt {
  if (!ownsConnection && announce) {
    throw new Error('Use the main Jaz window to connect')
  }
  invalidate()
  if (announce && bridge && desktop) {
    authoritative = bridge.update({ operation: 'begin', document: desktop.document, url })
  }
  const value = { url, revision: authoritative?.revision ?? 0, controller: new AbortController() }
  attempt = value
  setApiBaseUrl(url)
  setState({ status: 'checking', url, pairing: null, error: null })
  return value
}

function block(error: string | null): void {
  invalidate()
  if (ownsConnection) {
    clearConnectionPreference()
    if (bridge && desktop) {
      authoritative = bridge.update({ operation: 'clear', document: desktop.document, error })
    }
    void clientRuntime.configureThreadNotifications?.({ enabled: false })
  } else if (bridge && desktop && authoritative?.phase === 'connected') {
    authoritative = bridge.update({ operation: 'reject', document: desktop.document, revision: authoritative.revision })
  }
  setState({ status: 'disconnected', pairing: null, error })
}

function activate(value: Attempt, candidate: Candidate): void {
  if (!current(value)) {
    return
  }
  if (ownsConnection && bridge && desktop) {
    authoritative = bridge.update({ operation: 'commit', document: desktop.document, revision: value.revision,
      session: { url: candidate.url, token: candidate.token, deviceId: candidate.deviceId, remembered: candidate.remembered } })
    value.revision = authoritative.revision
  }
  setApiAuthToken(candidate.url, candidate.token, candidate.ticket)
  setState({ status: 'connected', url: candidate.url, pairing: null, error: null })
  if (ownsConnection) {
    if (candidate.url === LOCAL_URL && clientRuntime.kind === 'electron') {
      savePreference({ mode: 'local' })
    } else {
      clearConnectionPreference()
      rememberBackend(candidate.url, new Date().toISOString())
      if (candidate.remembered) {
        savePreference({ mode: 'remote', remoteUrl: candidate.url })
      }
    }
    schedulePoll(value, candidate)
  }
}

async function verify(value: Attempt, candidate: Candidate): Promise<string | null> {
  const access = await readAccess(value.url, candidate.token, value.controller.signal, candidate.deviceId || undefined)
  if (!current(value)) {
    return null
  }
  if (!access.ok || access.kind !== 'device') {
    const error = access.ok ? 'A device credential is required' : access.error
    if (!access.ok && access.recoverable) {
      setState({ status: 'disconnected', error })
      pollTimer = setTimeout(() => void verify(value, candidate), 5000)
      return error
    }
    block(error)
    return error
  }
  activate(value, { ...candidate, deviceId: access.deviceId, ticket: access.ticket })
  return null
}

function schedulePoll(value: Attempt, candidate: Candidate): void {
  clearTimeout(pollTimer)
  pollTimer = setTimeout(async () => {
    if (!current(value)) {
      return
    }
    const health = await readHealth(value.url, value.controller.signal)
    if (!current(value)) {
      return
    }
    const access = health.ok && candidate.token ? await readAccess(value.url, candidate.token, value.controller.signal, candidate.deviceId) : health
    if (!current(value)) {
      return
    }
    if (!access.ok) {
      if (!access.recoverable) {
        block(access.error)
        return
      }
      setState({ status: 'reconnecting', error: access.error })
    } else {
      setState({ status: 'connected', error: null })
    }
    schedulePoll(value, candidate)
  }, 5000)
}

async function serialized<T>(value: Attempt, issue: () => Promise<T>): Promise<T | undefined> {
  const next = issuance.catch(() => {}).then(() => current(value) ? issue() : undefined)
  issuance = next
  return next
}

async function register(value: Attempt, credential: string): Promise<string | null> {
  const profile = await getDeviceProfile()
  if (!current(value)) {
    return null
  }
  const response = await serialized(value, () => fetch(value.url + '/v1/devices/register', {
    method: 'POST', redirect: 'error',
    headers: { Authorization: `Bearer ${credential}`, 'Content-Type': 'application/json', [CLIENT_PLATFORM_HEADER]: CLIENT_PLATFORM },
    body: JSON.stringify({ ...profile, kind: clientRuntime.deviceKind }), signal: AbortSignal.timeout(5000),
  }))
  if (!response || !current(value)) {
    return null
  }
  const body = await readJSON<{ token?: string; error?: string; pairing?: { id: string; expires_at: string }; pairing_secret?: string }>(response)
  if (!current(value)) {
    return null
  }
  if (response.ok && body?.token) {
    return verify(value, { url: value.url, token: body.token, deviceId: profile.device_id, remembered: false })
  }
  if (response.status === 202 && body?.pairing && body.pairing_secret) {
    pending(value, { url: value.url, id: body.pairing.id, secret: body.pairing_secret, expiresAt: body.pairing.expires_at, deviceName: profile.name }, profile.device_id)
    return null
  }
  const error = body?.error || 'Device registration failed'
  block(error)
  return error
}

function pending(value: Attempt, pairing: PendingPairing, deviceId: string): void {
  if (!current(value)) {
    return
  }
  setState({ status: 'pending_approval', pairing })
  const poll = async (): Promise<void> => {
    if (!current(value)) {
      return
    }
    try {
      const response = await fetch(value.url + '/v1/devices/pairing-requests/' + encodeURIComponent(pairing.id), {
        headers: { [CLIENT_PLATFORM_HEADER]: CLIENT_PLATFORM, 'X-Jaz-Pairing-Secret': pairing.secret },
        signal: AbortSignal.any([value.controller.signal, AbortSignal.timeout(5000)]),
      })
      const body = await readJSON<{ token?: string; error?: string; pairing?: { status: string } }>(response)
      if (!current(value)) {
        return
      }
      if (response.ok && body?.pairing?.status === 'approved' && body.token) {
        await verify(value, { url: value.url, token: body.token, deviceId, remembered: false })
        return
      }
      if (!response.ok || body?.pairing?.status === 'rejected' || body?.pairing?.status === 'expired') {
        block(body?.error || 'Device approval is no longer available. Connect again.')
        return
      }
    } catch {
      if (!current(value)) {
        return
      }
    }
    pairingTimer = setTimeout(() => void poll(), 2000)
  }
  pairingTimer = setTimeout(() => void poll(), 2000)
}

async function pair(value: Attempt): Promise<string | null> {
  const profile = await getDeviceProfile()
  if (!current(value)) {
    return null
  }
  const response = await fetch(value.url + '/v1/devices/pairing-requests', {
    method: 'POST', redirect: 'error', headers: { 'Content-Type': 'application/json', [CLIENT_PLATFORM_HEADER]: CLIENT_PLATFORM },
    body: JSON.stringify({ ...profile, kind: clientRuntime.deviceKind }), signal: AbortSignal.any([value.controller.signal, AbortSignal.timeout(5000)]),
  })
  const body = await readJSON<{ pairing?: { id: string; expires_at: string }; pairing_secret?: string; error?: string }>(response)
  if (!current(value)) {
    return null
  }
  if (!response.ok || !body?.pairing || !body.pairing_secret) {
    const error = body?.error || 'Could not create a device approval request'
    block(error)
    return error
  }
  pending(value, { url: value.url, id: body.pairing.id, secret: body.pairing_secret, expiresAt: body.pairing.expires_at, deviceName: profile.name }, profile.device_id)
  return null
}

export async function connectRemote(input: string): Promise<string | null> {
  const parsed = parseBackendConnectUrl(input)
  const url = normalizeBaseUrl(parsed.url)
  if (!secureBackendConnection(url)) {
    return 'Remote connections require HTTPS'
  }
  const cached = cachedApiAuthToken(url)
  let remembered = !parsed.key && authoritative?.session?.url === url && authoritative.session.token === cached && authoritative.session.remembered === true
  const value = begin(url)
  clearConnectionPreference()
  try {
    if (parsed.key.startsWith('invite_')) {
      return await register(value, parsed.key)
    }
    const health = await readHealth(url, value.controller.signal)
    if (!current(value)) {
      return null
    }
    if (!health.ok) {
      block(health.error)
      return health.error
    }
    if (!health.authRequired) {
      activate(value, { url, token: '', deviceId: '', remembered: false })
      return null
    }
    let token = parsed.key || cached
    if (!token) {
      token = restoreApiAuthToken(url)
      remembered = Boolean(token)
    }
    if (!token) {
      return await pair(value)
    }
    const access = await readAccess(url, token, value.controller.signal)
    if (!current(value)) {
      return null
    }
    if (access.ok && access.kind === 'root') {
      return await register(value, token)
    }
    if (!access.ok) {
      if (access.code === 'device_approval_required' && parsed.key) {
        return await register(value, token)
      }
      block(access.error)
      return access.error
    }
    activate(value, { url, token, deviceId: access.deviceId, ticket: access.ticket, remembered })
    return null
  } catch (error) {
    if (!current(value)) {
      return null
    }
    const message = error instanceof Error ? error.message : 'Could not connect'
    block(message)
    return message
  }
}

export async function startLocal(): Promise<string | null> {
  const cached = cachedApiAuthToken(LOCAL_URL)
  const value = begin(LOCAL_URL)
  clearConnectionPreference()
  try {
    if (cached) {
      const local = await clientRuntime.startLocalBackend?.()
      if (!current(value)) {
        return null
      }
      if (!local?.ok || local.url !== LOCAL_URL) {
        throw new Error('Local backend did not start at its trusted origin')
      }
      return await verify(value, { url: LOCAL_URL, token: cached, deviceId: '', remembered: false })
    }
    if (!bridge || !desktop) {
      throw new Error('Local authenticated startup requires the desktop app')
    }
    const candidate = await bridge.local(desktop.document, value.revision)
    if (!current(value)) {
      return null
    }
    activate(value, candidate)
    return null
  } catch (error) {
    if (!current(value)) {
      return null
    }
    const message = error instanceof Error ? error.message : 'Could not start local Jaz'
    block(message)
    return message
  }
}

export function disconnectBackend(): void {
  block(null)
}

export function cancelPendingApproval(): void {
  block(null)
}

export function forgetBackend(url: string): void {
  if (normalizeBaseUrl(url) === state.url) {
    disconnectBackend()
  }
  removeKnownBackend(url)
  const preference = connectionPreference()
  if (preference?.mode === 'remote' && preference.remoteUrl === normalizeBaseUrl(url)) {
    clearConnectionPreference()
  }
}

export function rememberConnection(): string | null {
  if (!ownsConnection || state.status !== 'connected' || isLocalBackendUrl(state.url)) {
    return 'Connect to a remote server first'
  }
  const value = attempt
  const token = cachedApiAuthToken(state.url)
  try {
    rememberApiAuthToken(state.url, () => {
      if (!value || !current(value)) {
        return false
      }
      const active = bridge?.read()
      return !active || (active.document === desktop?.document && active.owner && active.snapshot.revision === value.revision)
    })
    if (!value || !current(value) || token !== cachedApiAuthToken(state.url)) {
      return 'Connection changed'
    }
    if (bridge && desktop) {
      authoritative = bridge.update({ operation: 'commit', document: desktop.document, revision: value.revision,
        session: { url: state.url, token, deviceId: authoritative?.session?.deviceId || '', remembered: true } })
      value.revision = authoritative.revision
    }
    savePreference({ mode: 'remote', remoteUrl: state.url })
    return null
  } catch {
    return 'Connected for this session. Could not remember this connection.'
  }
}

export async function checkHealth(url: string): Promise<boolean> {
  return (await readHealth(url)).ok
}

async function hydrate(snapshot: SessionSnapshot): Promise<void> {
  if (authoritative && snapshot.revision < authoritative.revision) {
    return
  }
  if (snapshot.phase === 'connected' && snapshot.session && state.status === 'connected' &&
    snapshot.url === state.url && snapshot.session.token === cachedApiAuthToken(state.url) && attempt) {
    authoritative = snapshot
    attempt.revision = snapshot.revision
    return
  }
  authoritative = snapshot
  invalidate()
  if (snapshot.phase === 'connected' && snapshot.session) {
    const value = begin(snapshot.url, false)
    if (snapshot.session.token) {
      await verify(value, snapshot.session)
    } else {
      activate(value, snapshot.session)
    }
  } else if (ownsConnection && snapshot.phase === 'pending' && snapshot.localPending) {
    await startLocal()
  }
  else {
    if (ownsConnection && snapshot.phase === 'blocked') {
      clearConnectionPreference()
    }
    setState({ status: snapshot.phase === 'pending' || snapshot.phase === 'initial' ? 'checking' : 'disconnected', url: snapshot.url || state.url, pairing: null, error: snapshot.error })
  }
}

async function init(): Promise<void> {
  if (desktop && desktop.snapshot.phase !== 'initial') {
    await hydrate(desktop.snapshot)
    return
  }
  if (!ownsConnection) {
    return
  }
  const startup = consumeStartupConnectUrl()
  if (startup) {
    await connectRemote(startup)
    return
  }
  const preference = connectionPreference()
  if (preference?.mode === 'local') {
    await startLocal()
    return
  }
  if (preference?.mode === 'remote' && preference.remoteUrl) {
    await restore(preference.remoteUrl)
    return
  }
  const url = normalizeBaseUrl(localBaseUrl())
  const value = begin(url)
  const health = await readHealth(url, value.controller.signal)
  if (!current(value)) {
    return
  }
  if (health.ok && !health.authRequired) {
    activate(value, { url, token: '', deviceId: '', remembered: false })
  }
  else {
    block(null)
  }
}

async function restore(url: string): Promise<void> {
  const value = begin(url)
  let token: string | undefined
  const retry = async (): Promise<void> => {
    const health = await readHealth(url, value.controller.signal)
    if (!current(value)) {
      return
    }
    if (!health.ok) {
      if (health.recoverable) {
        setState({ status: 'disconnected', error: health.error })
        pollTimer = setTimeout(() => void retry(), 5000)
      } else {
        block(health.error)
      }
      return
    }
    if (!health.authRequired) {
      activate(value, { url, token: '', deviceId: '', remembered: false })
      return
    }
    try {
      token ??= restoreApiAuthToken(url)
    } catch {
      block('Could not unlock the saved connection. Allow Jaz access to your system keychain, then restart Jaz.')
      return
    }
    if (!token) {
      block('Connect again with an authorized link or request approval')
      return
    }
    const access = await readAccess(url, token, value.controller.signal)
    if (!current(value)) {
      return
    }
    if (!access.ok) {
      if (access.recoverable) {
        pollTimer = setTimeout(() => void retry(), 5000)
      }
      else {
        block(access.error)
      }
      return
    }
    if (access.kind !== 'device') {
      block('A saved device credential is required')
      return
    }
    activate(value, { url, token, deviceId: access.deviceId, ticket: access.ticket, remembered: true })
  }
  await retry()
}

bridge?.subscribe((snapshot) => {
  void hydrate(snapshot)
})
onApiAuthRejected(() => block('Backend device access was rejected'))
void init().catch((error: unknown) => block(error instanceof Error ? error.message : 'Could not open the connection'))
