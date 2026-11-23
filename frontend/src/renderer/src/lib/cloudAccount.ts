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
  post,
  secureBackendConnection,
} from '@/lib/api/client'
import type { CloudUser, ServerCapabilities } from '@/lib/api/serverTypes'
import { clientRuntime } from '@/lib/clientRuntime'
import { fetchServerDiscovery } from '@/lib/connectionProbe'
import { keys, WORKSPACE_QUERY_ROOTS } from '@/lib/query/keys'
import { queryClient } from '@/lib/query/queryClient'

export const LOCAL_WORKSPACE_ID = 'local'

export type CloudAccountStatus = 'disconnected' | 'checking' | 'connected'

export interface CloudWorkspaceSummary {
  id: string
  url: string
  org_id?: string
  org_name?: string
  email?: string
  display_name?: string
  role?: string
  oidc_login_url?: string
  capabilities?: ServerCapabilities
  connected_at?: string
  last_sync_at?: string
  last_error?: string
}

export interface CloudAccountState {
  status: CloudAccountStatus
  activeWorkspaceId: string
  workspaces: CloudWorkspaceSummary[]
  url: string
  user: CloudUser | null
  error: string | null
  oidcLoginUrl: string
  capabilities: ServerCapabilities
  lastSyncAt: string
}

export interface BackendCloudStatus {
  active_workspace_id?: string
  connected: boolean
  status: string
  url?: string
  user?: CloudUser | null
  oidc_login_url?: string
  capabilities?: ServerCapabilities
  connected_at?: string
  last_sync_at?: string
  last_error?: string
  workspaces?: CloudWorkspaceSummary[]
}

const EMPTY_STATE: CloudAccountState = {
  status: 'disconnected',
  activeWorkspaceId: LOCAL_WORKSPACE_ID,
  workspaces: [],
  url: '',
  user: null,
  error: null,
  oidcLoginUrl: '',
  capabilities: {},
  lastSyncAt: '',
}

let state: CloudAccountState = { ...EMPTY_STATE }
let statusHydrated = false
let attemptSeq = 0
let activeController: AbortController | null = null
const listeners = new Set<() => void>()
const workspaceChangeListeners = new Set<(workspaceId: string) => void>()

function setState(next: Partial<CloudAccountState>): void {
  state = { ...state, ...next }
  for (const listener of listeners) {
    listener()
  }
}

function notifyWorkspaceChange(workspaceId: string): void {
  for (const listener of workspaceChangeListeners) {
    listener(workspaceId)
  }
}

export function onWorkspaceChange(listener: (workspaceId: string) => void): () => void {
  workspaceChangeListeners.add(listener)
  return () => workspaceChangeListeners.delete(listener)
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
  const workspaces = Array.isArray(res.workspaces) ? res.workspaces : []
  if (!res.connected) {
    setState({
      status: 'disconnected',
      activeWorkspaceId: res.active_workspace_id || LOCAL_WORKSPACE_ID,
      workspaces,
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
    activeWorkspaceId: res.active_workspace_id || workspaces[0]?.id || '',
    workspaces,
    url: res.url ?? '',
    user: res.user ?? null,
    error: res.last_error || null,
    oidcLoginUrl: res.oidc_login_url || '/v1/auth/login',
    capabilities: res.capabilities ?? {},
    lastSyncAt: res.last_sync_at ?? '',
  })
}

function refreshWorkspaceQueries({ reset = false, clearCloud = false, refetchCloud = true } = {}): void {
  if (clearCloud || reset) {
    void queryClient.cancelQueries({ queryKey: keys.cloud })
    queryClient.removeQueries({ queryKey: keys.cloud })
  }
  if (refetchCloud) void queryClient.invalidateQueries({ queryKey: keys.cloud })
  for (const queryKey of WORKSPACE_QUERY_ROOTS) {
    if (reset) {
      void queryClient.cancelQueries({ queryKey })
      queryClient.removeQueries({ queryKey })
    }
    void queryClient.invalidateQueries({ queryKey })
  }
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

function commitBackendStatus(
  res: BackendCloudStatus,
  prevWorkspaceId: string,
  options: {
    forceRefresh?: boolean
    prevConnected?: boolean
    clearCloudOnDisconnect?: boolean
    notifyIfHydrated?: boolean
  } = {},
): void {
  const wasHydrated = statusHydrated
  statusHydrated = true
  applyBackendStatus(res)
  const workspaceChanged = prevWorkspaceId !== state.activeWorkspaceId
  const connectionChanged = options.prevConnected !== undefined && res.connected !== options.prevConnected
  if (options.forceRefresh || workspaceChanged || connectionChanged || (options.clearCloudOnDisconnect && !res.connected)) {
    refreshWorkspaceQueries({
      reset: workspaceChanged,
      clearCloud: Boolean(options.clearCloudOnDisconnect && !res.connected) || workspaceChanged,
      refetchCloud: res.connected,
    })
  }
  if (workspaceChanged && (!options.notifyIfHydrated || wasHydrated)) {
    notifyWorkspaceChange(state.activeWorkspaceId)
  }
}

export function resetCloudAccountState(): void {
  activeController?.abort()
  activeController = null
  attemptSeq += 1
  statusHydrated = false
  setState({ ...EMPTY_STATE })
}

export async function refreshCloudAccountStatus(): Promise<void> {
  if (!clientRuntime.capabilities.localBackend || isCloudApiSession() || !apiBaseUrl()) return
  const prevConnected = state.status === 'connected'
  const prevWorkspaceId = state.activeWorkspaceId
  const { seq, controller } = beginAttempt()
  try {
    const res = await get<BackendCloudStatus>('/v1/cloud/status', { signal: controller.signal })
    if (!isCurrent(seq, controller)) return
    commitBackendStatus(res, prevWorkspaceId, {
      prevConnected,
      clearCloudOnDisconnect: true,
      notifyIfHydrated: true,
    })
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
  setState(
    state.status === 'connected'
      ? { error: null }
      : { url: origin, error: null, oidcLoginUrl: loginPath },
  )
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
      setState({
        ...(state.status !== 'connected' ? { status: 'disconnected' } : {}),
        error: validated.error,
      })
    }
    return validated.error
  }

  const wasConnected = state.status === 'connected'
  const prevWorkspaceId = state.activeWorkspaceId
  const { seq, controller } = beginAttempt()
  setState(wasConnected ? { error: null } : { status: 'checking', url: validated.server, error: null })
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
    commitBackendStatus(res, prevWorkspaceId, { forceRefresh: true })
    return null
  } catch (error) {
    if (!isCurrent(seq, controller)) return null
    const message = error instanceof Error ? error.message : 'Jaz Cloud sign-in failed'
    setState({
      ...(wasConnected ? {} : { status: 'disconnected' }),
      error: message,
    })
    return message
  }
}

export async function switchWorkspace(workspaceId: string): Promise<string | null> {
  if (!clientRuntime.capabilities.localBackend || isCloudApiSession() || !apiBaseUrl()) {
    return 'Workspace switching is unavailable in this session'
  }
  const prevWorkspaceId = state.activeWorkspaceId
  const { seq, controller } = beginAttempt()
  try {
    const res = await post<BackendCloudStatus>(
      '/v1/cloud/switch',
      { workspace_id: workspaceId },
      controller.signal,
    )
    if (!isCurrent(seq, controller)) return null
    commitBackendStatus(res, prevWorkspaceId, { forceRefresh: true, clearCloudOnDisconnect: true })
    return null
  } catch (error) {
    if (!isCurrent(seq, controller)) return null
    const message = error instanceof Error ? error.message : 'Could not switch workspace'
    setState({ error: message })
    return message
  }
}

export async function syncCloudAccount(): Promise<string | null> {
  const prevWorkspaceId = state.activeWorkspaceId
  const { seq, controller } = beginAttempt()
  try {
    const res = await post<BackendCloudStatus>('/v1/cloud/sync', undefined, controller.signal)
    if (!isCurrent(seq, controller)) return null
    commitBackendStatus(res, prevWorkspaceId, { forceRefresh: true, clearCloudOnDisconnect: true })
    return null
  } catch (error) {
    if (!isCurrent(seq, controller)) return null
    const message = error instanceof Error ? error.message : 'Cloud sync failed'
    setState({ error: message })
    return message
  }
}

export async function signOutCloudAccount(workspaceId?: string): Promise<void> {
  const prevWorkspaceId = state.activeWorkspaceId
  const { seq, controller } = beginAttempt()
  if (!clientRuntime.capabilities.localBackend || isCloudApiSession() || !apiBaseUrl()) {
    setState({ ...EMPTY_STATE })
    refreshWorkspaceQueries({ reset: true, clearCloud: true, refetchCloud: false })
    if (prevWorkspaceId !== state.activeWorkspaceId) notifyWorkspaceChange(state.activeWorkspaceId)
    return
  }
  try {
    const res = await post<BackendCloudStatus>(
      '/v1/cloud/disconnect',
      workspaceId ? { workspace_id: workspaceId } : undefined,
      controller.signal,
    )
    if (!isCurrent(seq, controller)) return
    commitBackendStatus(res, prevWorkspaceId, { clearCloudOnDisconnect: true })
  } catch {
    if (!isCurrent(seq, controller)) return
    const remaining = workspaceId ? state.workspaces.filter((ws) => ws.id !== workspaceId) : []
    if (!workspaceId || workspaceId === state.activeWorkspaceId) {
      setState({ ...EMPTY_STATE, workspaces: remaining })
      refreshWorkspaceQueries({ reset: true, clearCloud: true, refetchCloud: false })
      if (prevWorkspaceId !== state.activeWorkspaceId) notifyWorkspaceChange(state.activeWorkspaceId)
    } else {
      setState({ workspaces: remaining })
    }
  }
}
