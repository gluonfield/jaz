import { useSyncExternalStore } from 'react'
import {
  beginPendingOidcLogin,
  consumePendingOidcLogin,
  launchOidcBrowserFlow,
  parseOidcCallbackUrl,
  validateOidcCallback,
} from '@/lib/api/cloudAuth'
import {
  apiBaseUrl,
  get,
  isCloudApiSession,
  normalizeBaseUrl,
  onApiSessionChange,
  post,
  secureBackendConnection,
} from '@/lib/api/client'
import type { CloudUser, ServerCapabilities } from '@/lib/api/serverTypes'
import { fetchServerDiscovery } from '@/lib/connectionProbe'
import { keys } from '@/lib/query/keys'
import { queryClient } from '@/lib/query/queryClient'

export type CloudAccountStatus = 'disconnected' | 'checking' | 'connected'

export interface CloudAccountState {
  status: CloudAccountStatus
  url: string
  user: CloudUser | null
  error: string | null
  oidcLoginUrl: string
  capabilities: ServerCapabilities
  lastSyncAt: string
}

interface BackendCloudStatus {
  connected: boolean
  status: string
  url?: string
  user?: CloudUser | null
  oidc_login_url?: string
  capabilities?: ServerCapabilities
  last_sync_at?: string
  last_error?: string
}

const EMPTY_STATE: CloudAccountState = {
  status: 'disconnected',
  url: '',
  user: null,
  error: null,
  oidcLoginUrl: '',
  capabilities: {},
  lastSyncAt: '',
}

let state: CloudAccountState = { ...EMPTY_STATE }
let attemptSeq = 0
let activeController: AbortController | null = null
const listeners = new Set<() => void>()

function setState(next: Partial<CloudAccountState>): void {
  state = { ...state, ...next }
  for (const listener of listeners) {
    listener()
  }
}

export function useCloudAccount(): CloudAccountState {
  return useSyncExternalStore(
    (listener) => {
      listeners.add(listener)
      return () => listeners.delete(listener)
    },
    () => state,
  )
}

export function currentCloudAccount(): CloudAccountState {
  return state
}

function applyBackendStatus(res: BackendCloudStatus): void {
  if (!res.connected) {
    setState({
      status: 'disconnected',
      url: res.url ?? '',
      user: null,
      error: res.last_error || null,
      oidcLoginUrl: res.oidc_login_url ?? '',
      capabilities: res.capabilities ?? {},
      lastSyncAt: res.last_sync_at ?? '',
    })
    return
  }
  setState({
    status: 'connected',
    url: res.url ?? '',
    user: res.user ?? null,
    error: res.last_error || null,
    oidcLoginUrl: res.oidc_login_url || '/v1/auth/login',
    capabilities: res.capabilities ?? {},
    lastSyncAt: res.last_sync_at ?? '',
  })
}

function invalidateCloudQueries(): void {
  void queryClient.invalidateQueries({ queryKey: ['cloud'] })
  void queryClient.invalidateQueries({ queryKey: keys.sidebarSessions })
  void queryClient.invalidateQueries({ queryKey: keys.usage })
  void queryClient.invalidateQueries({ queryKey: keys.agentFiles })
}

function removeCloudQueries(): void {
  queryClient.removeQueries({ queryKey: ['cloud'] })
  void queryClient.invalidateQueries({ queryKey: keys.sidebarSessions })
  void queryClient.invalidateQueries({ queryKey: keys.usage })
  void queryClient.invalidateQueries({ queryKey: keys.agentFiles })
}

function beginAttempt(): { seq: number; controller: AbortController } {
  activeController?.abort()
  const controller = new AbortController()
  activeController = controller
  attemptSeq += 1
  return { seq: attemptSeq, controller }
}

function isCurrent(seq: number, controller: AbortController): boolean {
  return attemptSeq === seq && !controller.signal.aborted
}

export async function refreshCloudAccountStatus(): Promise<void> {
  if (isCloudApiSession() || !apiBaseUrl()) return
  const { seq, controller } = beginAttempt()
  try {
    const res = await get<BackendCloudStatus>('/v1/cloud/status', { signal: controller.signal })
    if (!isCurrent(seq, controller)) return
    const wasConnected = state.status === 'connected'
    applyBackendStatus(res)
    if (res.connected && !wasConnected) {
      invalidateCloudQueries()
    }
  } catch {
    if (!isCurrent(seq, controller)) return
    setState({ ...EMPTY_STATE })
  }
}

export async function attachCloudSignIn(baseUrl: string, oidcLoginUrl?: string): Promise<string | null> {
  const origin = normalizeBaseUrl(baseUrl)
  if (!origin || !secureBackendConnection(origin)) {
    return 'Jaz Cloud connections require HTTPS'
  }
  let loginPath = oidcLoginUrl?.trim() ?? ''
  if (!loginPath) {
    const discovery = await fetchServerDiscovery(origin)
    if (!discovery.ok) return discovery.error
    if (discovery.mode !== 'cloud' && discovery.auth !== 'oidc') {
      return 'This server is not a Jaz Cloud workspace'
    }
    loginPath = discovery.oidcLoginUrl || '/v1/auth/login'
  }
  const pending = beginPendingOidcLogin(origin, Date.now, 'attached')
  setState({ url: origin, error: null, oidcLoginUrl: loginPath })
  await launchOidcBrowserFlow(origin, loginPath, pending)
  return null
}

export async function completeAttachedOidcCallback(
  rawCallbackUrl: string,
  options: { passive?: boolean } = {},
): Promise<string | null> {
  const parsed = parseOidcCallbackUrl(rawCallbackUrl)
  if (!parsed.code && !parsed.token) {
    return options.passive ? null : 'Callback did not include an authorization code or session token'
  }
  const pending = consumePendingOidcLogin()
  if (!pending && options.passive) {
    return null
  }
  const validated = validateOidcCallback(rawCallbackUrl, pending)
  if (!validated.ok) {
    if (!options.passive) {
      setState({ status: 'disconnected', error: validated.error })
    }
    return validated.error
  }

  const { seq, controller } = beginAttempt()
  setState({ status: 'checking', url: validated.server, error: null })
  try {
    const res = await post<BackendCloudStatus>(
      '/v1/cloud/connect',
      {
        url: validated.server,
        ...(validated.code ? { code: validated.code, code_verifier: validated.codeVerifier } : {}),
        ...(validated.token ? { token: validated.token } : {}),
      },
      controller.signal,
    )
    if (!isCurrent(seq, controller)) return null
    applyBackendStatus(res)
    invalidateCloudQueries()
    return null
  } catch (error) {
    if (!isCurrent(seq, controller)) return null
    const message = error instanceof Error ? error.message : 'Jaz Cloud sign-in failed'
    setState({ status: 'disconnected', error: message })
    return message
  }
}

export async function syncCloudAccount(): Promise<string | null> {
  try {
    const res = await post<BackendCloudStatus>('/v1/cloud/sync')
    applyBackendStatus(res)
    invalidateCloudQueries()
    return null
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Cloud sync failed'
    setState({ error: message })
    return message
  }
}

export function signOutCloudAccount(): void {
  activeController?.abort()
  activeController = null
  attemptSeq += 1
  setState({ ...EMPTY_STATE })
  removeCloudQueries()
  if (!isCloudApiSession() && apiBaseUrl()) {
    void post('/v1/cloud/disconnect').catch(() => {})
  }
}

onApiSessionChange(() => {
  if (isCloudApiSession()) {
    setState({ ...EMPTY_STATE })
    return
  }
  if (apiBaseUrl()) {
    void refreshCloudAccountStatus()
  }
})
