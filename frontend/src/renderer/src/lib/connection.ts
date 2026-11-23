import { useEffect, useRef, useSyncExternalStore } from 'react'
import {
  beginPendingOidcLogin,
  consumePendingOidcLogin,
  exchangeOidcCode,
  launchOidcBrowserFlow,
  logoutCloudSession,
  parseOidcCallbackUrl,
  peekPendingOidcLogin,
  validateOidcCallback,
} from '@/lib/api/cloudAuth'
import { completeAttachedOidcCallback, refreshCloudAccountStatus, resetCloudAccountState } from '@/lib/cloudAccount'
import type {
  CloudUser,
  ServerAuthMethod,
  ServerCapabilities,
  ServerMode,
} from '@/lib/api/serverTypes'
import {
  apiBaseUrl,
  cachedApiAuthToken,
  clearApiSession,
  isLocalBackendUrl,
  localBaseUrl,
  normalizeBaseUrl,
  onApiAuthRejected,
  parseBackendConnectUrl,
  rememberApiAuthToken,
  restoreApiAuthToken,
  secureBackendConnection,
  setApiAuthToken,
  setApiBaseUrl,
  setApiServerMode,
  setResourceTicket,
} from '@/lib/api/client'
import { rememberBackend, removeKnownBackend, updateBackendMetadata } from '@/lib/backends'
import { clientRuntime } from '@/lib/clientRuntime'
import { loadBrowserCredential, saveBrowserCredential, forgetBrowserCredential } from '@/lib/browserCredentials'
import { queryClient } from '@/lib/query/queryClient'
import { fetchServerDiscovery, readAccess, readCloudAccess, type ServerDiscovery } from '@/lib/connectionProbe'
import type { ResourceTicket, SessionSnapshot, VerifiedSession } from '@shared/connectionSession'
import { LOCAL_BACKEND_URL as LOCAL_URL } from '@shared/localBackend'

export { isLocalBackendUrl } from '@/lib/api/client'
export { fetchServerDiscovery, type ServerDiscovery } from '@/lib/connectionProbe'
export type ConnectionStatus = 'checking' | 'connected' | 'reconnecting' | 'disconnected'
export type ConnectionState = {
  status: ConnectionStatus
  url: string
  error: string | null
  mode: ServerMode
  auth: ServerAuthMethod
  oidcLoginUrl: string
  capabilities: ServerCapabilities
  user: CloudUser | null
}
export type ConnectionMode = 'local' | 'remote'
type ConnectionPreference = { mode: ConnectionMode; remoteUrl?: string }
type Attempt = { url: string; revision: number; controller: AbortController }
type ServerMeta = {
  mode: ServerMode
  auth: ServerAuthMethod
  oidcLoginUrl: string
  capabilities: ServerCapabilities
}
type Candidate = VerifiedSession & { ticket?: ResourceTicket; user?: CloudUser; meta?: ServerMeta }
type SavedCredential = { token: string }
type DiscoverySuccess = Extract<ServerDiscovery, { ok: true }>
type ProbeOptions = {
  explicitToken?: string
  resolveSaved?: () => Promise<SavedCredential>
  onSavedError?: () => string
  remembered: boolean
  allowWebCookie?: boolean
  retryOnRecoverable?: boolean
  useCachedDiscovery?: boolean
  fallbackMeta?: ServerMeta
  missingCredentialError?: (meta: ServerMeta) => string
}

const DEFAULT_META: ServerMeta = { mode: 'local', auth: 'none', oidcLoginUrl: '', capabilities: {} }
const CLOUD_FALLBACK_META: ServerMeta = { mode: 'cloud', auth: 'oidc', oidcLoginUrl: '/v1/auth/login', capabilities: {} }
const PREFERENCE_KEY = 'jaz.connection'
const bridge = ['main', 'board', 'launcher'].includes(clientRuntime.windowKind) ? clientRuntime.connectionSession : undefined
const desktop = bridge?.read()
export const ownsConnection = desktop?.owner ?? clientRuntime.windowKind === 'main'
let authoritative = desktop?.snapshot
let attempt: Attempt | null = null
let pollTimer: ReturnType<typeof setTimeout> | undefined
const discoveryCache = new Map<string, DiscoverySuccess>()
let state: ConnectionState = {
  status: 'checking',
  url: apiBaseUrl(),
  error: null,
  ...DEFAULT_META,
  user: null,
}
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
  }, () => state, () => state)
}

export function useIsCloud(): boolean {
  return useConnection().mode === 'cloud'
}

export function isCloudConnection(): boolean {
  return state.mode === 'cloud'
}

export function currentConnection(): ConnectionState {
  return state
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
  clearApiSession()
  void queryClient.cancelQueries()
  queryClient.clear()
}

function current(value: Attempt): boolean {
  return attempt === value && !value.controller.signal.aborted && (!authoritative || authoritative.revision === value.revision)
}

function discoveryMeta(discovery: DiscoverySuccess): ServerMeta {
  return {
    mode: discovery.mode,
    auth: discovery.auth,
    oidcLoginUrl: discovery.oidcLoginUrl,
    capabilities: discovery.capabilities,
  }
}

async function discoverServer(
  url: string,
  signal?: AbortSignal,
  options: { useCache?: boolean } = {},
): Promise<ServerDiscovery> {
  const origin = normalizeBaseUrl(url)
  if (options.useCache) {
    const cached = discoveryCache.get(origin)
    if (cached) return cached
  }
  const result = await fetchServerDiscovery(origin, signal)
  if (result.ok) {
    discoveryCache.set(origin, result)
    updateBackendMetadata(origin, {
      mode: result.mode,
      auth: result.auth,
      oidcLoginUrl: result.oidcLoginUrl,
    })
  }
  return result
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
  setState({ status: 'checking', url, error: null, ...DEFAULT_META, user: null })
  return value
}

function block(error: string | null, keepPreference = false, meta?: Partial<ServerMeta>): void {
  invalidate()
  resetCloudAccountState()
  if (ownsConnection) {
    if (!keepPreference) clearConnectionPreference()
    if (bridge && desktop) {
      authoritative = bridge.update({ operation: 'clear', document: desktop.document, error })
    }
    void clientRuntime.configureNotifications?.({ mode: 'off' })
  } else if (bridge && desktop && authoritative?.phase === 'connected') {
    authoritative = bridge.update({ operation: 'reject', document: desktop.document, revision: authoritative.revision })
  }
  setState({
    status: 'disconnected',
    error,
    ...(meta ? {
      mode: meta.mode ?? state.mode,
      auth: meta.auth ?? state.auth,
      oidcLoginUrl: meta.oidcLoginUrl ?? state.oidcLoginUrl,
      capabilities: meta.capabilities ?? state.capabilities,
    } : DEFAULT_META),
    user: null,
  })
}

function activate(value: Attempt, candidate: Candidate): void {
  if (!current(value)) {
    return
  }
  const meta = candidate.meta ?? DEFAULT_META
  if (ownsConnection && bridge && desktop) {
    authoritative = bridge.update({
      operation: 'commit',
      document: desktop.document,
      revision: value.revision,
      session: { url: candidate.url, token: candidate.token, remembered: candidate.remembered },
    })
    value.revision = authoritative.revision
  }
  setApiServerMode(meta.mode)
  setApiAuthToken(candidate.url, candidate.token, candidate.ticket)
  if (meta.mode === 'local' && clientRuntime.capabilities.localBackend) {
    void refreshCloudAccountStatus()
  } else {
    resetCloudAccountState()
  }
  setState({
    status: 'connected',
    url: candidate.url,
    error: null,
    mode: meta.mode,
    auth: meta.auth,
    oidcLoginUrl: meta.oidcLoginUrl,
    capabilities: meta.capabilities,
    user: candidate.user ?? null,
  })
  if (ownsConnection) {
    if (candidate.url === LOCAL_URL && clientRuntime.kind === 'electron') {
      savePreference({ mode: 'local' })
    } else {
      clearConnectionPreference()
      rememberBackend(candidate.url, new Date().toISOString(), {
        mode: meta.mode,
        auth: meta.auth,
        oidcLoginUrl: meta.oidcLoginUrl,
      })
      if (candidate.remembered) {
        savePreference({ mode: 'remote', remoteUrl: candidate.url })
      } else if (clientRuntime.kind === 'web' && candidate.token) {
        void saveBrowserCredential(candidate.url, candidate.token, () => current(value)).then(() => {
          if (current(value)) savePreference({ mode: 'remote', remoteUrl: candidate.url })
        }, () => {
          if (current(value)) setState({ error: 'Connected for this session. Could not save the login in this browser. Check site storage access.' })
        })
      }
    }
    schedulePoll(value, { ...candidate, meta })
  }
}

async function verifyCandidate(
  url: string,
  token: string,
  meta: ServerMeta,
  signal: AbortSignal,
) {
  if (meta.auth === 'oidc') {
    return readCloudAccess(url, token, signal)
  }
  return readAccess(url, token, signal)
}

async function savedCredential(url: string): Promise<SavedCredential> {
  if (clientRuntime.kind === 'web') {
    return await loadBrowserCredential(url) || { token: '' }
  }
  return { token: restoreApiAuthToken(url) }
}

async function probeAndVerify(value: Attempt, options: ProbeOptions): Promise<string | null> {
  let cachedSaved: SavedCredential | undefined

  const run = async (useCache: boolean): Promise<string | null> => {
    const discovery = await discoverServer(value.url, value.controller.signal, { useCache })
    if (!current(value)) {
      return null
    }
    if (!discovery.ok) {
      if (options.retryOnRecoverable && discovery.recoverable) {
        setState({ status: 'disconnected', error: discovery.error })
        pollTimer = setTimeout(() => void run(false), 5000)
        return discovery.error
      }
      if (!options.fallbackMeta) {
        block(discovery.error)
        return discovery.error
      }
    }

    const meta = discovery.ok ? discoveryMeta(discovery) : (options.fallbackMeta ?? DEFAULT_META)
    if (discovery.ok && !discovery.authRequired) {
      activate(value, { url: value.url, token: '', remembered: false, meta })
      return null
    }

    let token = options.explicitToken ?? ''
    let remembered = options.remembered
    if (!token && options.resolveSaved) {
      try {
        if (cachedSaved === undefined) {
          cachedSaved = await options.resolveSaved()
        }
        token = cachedSaved.token
        remembered = remembered || Boolean(token)
      } catch (error) {
        if (!current(value)) return null
        if (options.onSavedError) {
          const message = options.onSavedError()
          block(message, false, meta)
          return message
        }
        if (clientRuntime.kind !== 'web') {
          const message = error instanceof Error ? error.message : 'Could not unlock the saved connection.'
          block(message, false, meta)
          return message
        }
        cachedSaved = { token: '' }
        token = ''
      }
      if (!current(value)) return null
    }

    if (!token) {
      if (options.allowWebCookie && meta.auth === 'oidc' && clientRuntime.kind === 'web') {
        const cookieAccess = await readCloudAccess(value.url, '', value.controller.signal)
        if (!current(value)) return null
        if (cookieAccess.ok) {
          activate(value, {
            url: value.url,
            token: '',
            ticket: cookieAccess.ticket,
            user: cookieAccess.user,
            remembered,
            meta,
          })
          return null
        }
      }
      const error = options.missingCredentialError
        ? options.missingCredentialError(meta)
        : meta.auth === 'oidc'
          ? 'Sign in to connect to this server'
          : 'Authentication required to connect to this server'
      block(error, false, meta)
      return error
    }

    const access = await verifyCandidate(value.url, token, meta, value.controller.signal)
    if (!current(value)) {
      return null
    }
    if (!access.ok) {
      if (options.retryOnRecoverable && access.recoverable) {
        setState({ status: 'disconnected', error: access.error })
        pollTimer = setTimeout(() => void run(false), 5000)
        return access.error
      }
      block(access.error, false, meta)
      return access.error
    }
    activate(value, {
      url: value.url,
      token,
      ticket: access.ticket,
      user: access.user,
      remembered,
      meta,
    })
    return null
  }

  return run(Boolean(options.useCachedDiscovery))
}

// A single missed probe is usually a busy client or backend, not an outage.
function schedulePoll(value: Attempt, candidate: Candidate, missed = false): void {
  clearTimeout(pollTimer)
  pollTimer = setTimeout(async () => {
    if (!current(value)) {
      return
    }
    const discovery = await discoverServer(value.url, value.controller.signal)
    if (!current(value)) {
      return
    }
    const meta = discovery.ok ? discoveryMeta(discovery) : (candidate.meta ?? DEFAULT_META)
    const needsAuthCheck = discovery.ok && (candidate.token || meta.auth === 'oidc')
    const access = needsAuthCheck
      ? await verifyCandidate(value.url, candidate.token, meta, value.controller.signal)
      : discovery
    if (!current(value)) {
      return
    }
    if (!access.ok) {
      if (!access.recoverable) {
        block(access.error, false, meta)
        return
      }
      if (missed) {
        setState({ status: 'reconnecting', error: access.error })
      }
    } else {
      if ('ticket' in access && access.ticket) {
        setResourceTicket(value.url, access.ticket)
      }
      const nextUser = 'user' in access && access.user ? access.user : state.user
      setState({ status: 'connected', error: null, user: nextUser })
    }
    schedulePoll(value, { ...candidate, meta }, !access.ok)
  }, 5000)
}

function consumeWebOidcCallback(): string | null {
  if (clientRuntime.kind !== 'web') return null
  const hash = window.location.hash
  if (!hash) return null
  const parsed = parseOidcCallbackUrl(hash)
  if (!parsed.code && !parsed.token) return null
  try {
    const clean = new URL(window.location.href)
    clean.hash = ''
    window.history.replaceState(window.history.state, '', clean.toString())
  } catch {
    // Clearing the fragment is best-effort.
  }
  return hash
}

export async function startCloudSignIn(baseUrl: string, oidcLoginUrl?: string): Promise<string | null> {
  const origin = normalizeBaseUrl(baseUrl)
  if (!secureBackendConnection(origin)) {
    return 'Remote connections require HTTPS'
  }
  let loginPath = oidcLoginUrl?.trim() ?? ''
  if (!loginPath) {
    const discovery = await discoverServer(origin, undefined, { useCache: true })
    if (!discovery.ok) return discovery.error
    if (discovery.auth !== 'oidc') {
      return connectRemote(origin)
    }
    loginPath = discovery.oidcLoginUrl || '/v1/auth/login'
  }
  const pending = beginPendingOidcLogin(origin, Date.now, 'primary')
  await launchOidcBrowserFlow(origin, loginPath, pending)
  return null
}

export async function completeOidcCallback(
  rawCallbackUrl: string,
  options: { passive?: boolean } = {},
): Promise<string | null> {
  const parsed = parseOidcCallbackUrl(rawCallbackUrl)
  if (!parsed.code && !parsed.token) {
    return options.passive ? null : 'Callback did not include an authorization code or session token'
  }
  const peeked = peekPendingOidcLogin()
  if (peeked?.purpose === 'attached') {
    return completeAttachedOidcCallback(rawCallbackUrl, options)
  }
  const pending = consumePendingOidcLogin()
  if (!pending && options.passive) {
    return null
  }
  const validated = validateOidcCallback(rawCallbackUrl, pending)
  if (!validated.ok) {
    if (!options.passive) {
      block(validated.error)
    }
    return validated.error
  }

  const value = begin(validated.server)
  clearConnectionPreference()
  try {
    if (validated.code) {
      const exchanged = await exchangeOidcCode(validated.server, validated.code, value.controller.signal, validated.codeVerifier)
      if (!current(value)) return null
      const discovery = await discoverServer(validated.server, value.controller.signal, { useCache: true })
      if (!current(value)) return null
      const meta = discovery.ok ? discoveryMeta(discovery) : CLOUD_FALLBACK_META
      activate(value, {
        url: validated.server,
        token: exchanged.token,
        ticket: exchanged.ticket,
        user: exchanged.user,
        remembered: false,
        meta,
      })
      return null
    }
    return await probeAndVerify(value, {
      explicitToken: validated.token,
      remembered: false,
      allowWebCookie: false,
      retryOnRecoverable: false,
      useCachedDiscovery: true,
      fallbackMeta: CLOUD_FALLBACK_META,
    })
  } catch (error) {
    if (!current(value)) return null
    const message = error instanceof Error ? error.message : 'Cloud sign-in failed'
    block(message)
    return message
  }
}

export async function connectRemote(input: string): Promise<string | null> {
  const parsed = parseBackendConnectUrl(input)
  const url = normalizeBaseUrl(parsed.url)
  if (!clientRuntime.capabilities.localBackend && isLocalBackendUrl(url)) {
    return 'Local backend is disabled for this build'
  }
  if (!secureBackendConnection(url)) {
    return 'Remote connections require HTTPS'
  }
  const cached = cachedApiAuthToken(url)
  const remembered = !parsed.key && authoritative?.session?.url === url && authoritative.session.token === cached && authoritative.session.remembered === true
  const value = begin(url)
  clearConnectionPreference()
  try {
    return await probeAndVerify(value, {
      explicitToken: parsed.key || cached,
      resolveSaved: () => savedCredential(url),
      remembered,
      allowWebCookie: true,
      retryOnRecoverable: false,
    })
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
  if (!clientRuntime.capabilities.localBackend) {
    return 'Local backend is disabled for this build'
  }
  const cached = cachedApiAuthToken(LOCAL_URL)
  const value = begin(LOCAL_URL)
  if (connectionPreference()?.mode !== 'local') clearConnectionPreference()
  try {
    if (cached) {
      const local = await clientRuntime.startLocalBackend?.()
      if (!current(value)) {
        return null
      }
      if (!local?.ok || local.url !== LOCAL_URL) {
        throw new Error('Local backend did not start at its trusted origin')
      }
      return await probeAndVerify(value, {
        explicitToken: cached,
        remembered: false,
        retryOnRecoverable: true,
        fallbackMeta: DEFAULT_META,
      })
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
    block(message, true)
    return message
  }
}

export function disconnectBackend(): void {
  if (state.status === 'connected' && state.mode === 'cloud' && state.auth === 'oidc' && state.url) {
    void logoutCloudSession(state.url, cachedApiAuthToken(state.url))
  }
  block(null)
}

export function forgetBackend(url: string): void {
  const origin = normalizeBaseUrl(url)
  discoveryCache.delete(origin)
  if (origin === state.url) {
    disconnectBackend()
  }
  if (clientRuntime.kind === 'web') {
    void forgetBrowserCredential(origin).catch(() => setState({ error: 'Could not remove the saved browser login. Check site storage access.' }))
  }
  removeKnownBackend(url)
  const preference = connectionPreference()
  if (preference?.mode === 'remote' && preference.remoteUrl === origin) {
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
      authoritative = bridge.update({
        operation: 'commit',
        document: desktop.document,
        revision: value.revision,
        session: { url: state.url, token, remembered: true },
      })
      value.revision = authoritative.revision
    }
    savePreference({ mode: 'remote', remoteUrl: state.url })
    return null
  } catch {
    return 'Connected for this session. Could not remember this connection.'
  }
}

export async function checkHealth(url: string): Promise<boolean> {
  return (await discoverServer(url)).ok
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
  const wasConnected = authoritative?.phase === 'connected'
  authoritative = snapshot
  invalidate()
  if (snapshot.phase === 'connected' && snapshot.session) {
    const value = begin(snapshot.url, false)
    if (snapshot.session.token) {
      await probeAndVerify(value, {
        explicitToken: snapshot.session.token,
        remembered: snapshot.session.remembered,
        retryOnRecoverable: true,
        useCachedDiscovery: true,
        fallbackMeta: DEFAULT_META,
      })
    } else {
      activate(value, snapshot.session)
    }
  } else if (ownsConnection && snapshot.phase === 'pending' && snapshot.localPending) {
    await startLocal()
  } else {
    if (ownsConnection && snapshot.phase === 'blocked' && (wasConnected || snapshot.url !== LOCAL_URL)) {
      clearConnectionPreference()
    }
    setState({
      status: snapshot.phase === 'pending' || snapshot.phase === 'initial' ? 'checking' : 'disconnected',
      url: snapshot.url || state.url,
      error: snapshot.error,
      ...DEFAULT_META,
      user: null,
    })
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
  const webCallback = consumeWebOidcCallback()
  const pendingOidc = webCallback ? peekPendingOidcLogin() : null
  const deferredAttachedCallback = webCallback && pendingOidc?.purpose === 'attached' ? webCallback : null
  if (webCallback && pendingOidc && pendingOidc.purpose !== 'attached') {
    await completeOidcCallback(webCallback, { passive: true })
    return
  }
  const preference = connectionPreference()
  if (preference?.mode === 'remote' && preference.remoteUrl) {
    await restore(preference.remoteUrl)
  } else if (clientRuntime.capabilities.localBackend) {
    await startLocal()
  } else if (clientRuntime.cloudUrl) {
    await connectRemote(clientRuntime.cloudUrl)
  } else if (clientRuntime.kind === 'electron') {
    block(null)
  } else {
    const url = normalizeBaseUrl(localBaseUrl())
    const value = begin(url)
    const discovery = await discoverServer(url, value.controller.signal)
    if (!current(value)) {
      return
    }
    if (discovery.ok && !discovery.authRequired) {
      activate(value, { url, token: '', remembered: false, meta: discoveryMeta(discovery) })
    } else {
      block(null, false, discovery.ok ? discoveryMeta(discovery) : undefined)
    }
  }
  if (deferredAttachedCallback && state.status === 'connected' && state.mode === 'local') {
    void completeAttachedOidcCallback(deferredAttachedCallback, { passive: true })
  }
}

async function restore(url: string): Promise<void> {
  const value = begin(url)
  await probeAndVerify(value, {
    resolveSaved: () => savedCredential(url),
    onSavedError: () => clientRuntime.kind === 'web'
      ? 'Could not restore the saved browser login. Check site storage access, then connect again.'
      : 'Could not unlock the saved connection. Allow Jaz access to your system keychain, then restart Jaz.',
    remembered: true,
    allowWebCookie: true,
    retryOnRecoverable: true,
    missingCredentialError: (meta) => meta.auth === 'oidc'
      ? 'Sign in to connect to this server'
      : 'Connect again with an authorized credential',
  })
}

if (clientRuntime.kind === 'web' && ownsConnection) {
  window.addEventListener('hashchange', () => {
    const incoming = consumeWebOidcCallback()
    if (incoming && peekPendingOidcLogin()) {
      void completeOidcCallback(incoming, { passive: true })
    }
  })
}

if (clientRuntime.kind === 'electron' && ownsConnection && clientRuntime.onOidcCallback) {
  clientRuntime.onOidcCallback((rawUrl) => {
    void completeOidcCallback(rawUrl)
  })
}

bridge?.subscribe((snapshot) => {
  void hydrate(snapshot)
})
onApiAuthRejected(() => block('Backend authentication was rejected'))
void init().catch((error: unknown) => block(error instanceof Error ? error.message : 'Could not open the connection'))
