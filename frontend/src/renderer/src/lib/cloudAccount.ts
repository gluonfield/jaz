import { useSyncExternalStore } from 'react'
import {
  beginPendingOidcLogin,
  consumePendingOidcLogin,
  exchangeOidcCode,
  launchOidcBrowserFlow,
  logoutCloudSession,
  parseOidcCallbackUrl,
  validateOidcCallback,
} from '@/lib/api/cloudAuth'
import {
  attachedCloudTarget,
  clearCloudThreads,
  onCloudAuthRejected,
  setAttachedCloudSession,
} from '@/lib/api/cloudClient'
import { normalizeBaseUrl, secureBackendConnection } from '@/lib/api/client'
import type { CloudUser, ServerCapabilities } from '@/lib/api/serverTypes'
import { forgetBrowserCredential, loadBrowserCredential, saveBrowserCredential } from '@/lib/browserCredentials'
import { clientRuntime } from '@/lib/clientRuntime'
import { fetchServerDiscovery, readCloudAccess } from '@/lib/connectionProbe'
import { keys } from '@/lib/query/keys'
import { queryClient } from '@/lib/query/queryClient'
import type { ResourceTicket } from '@shared/connectionSession'

export type CloudAccountStatus = 'disconnected' | 'checking' | 'connected'

export interface CloudAccountState {
  status: CloudAccountStatus
  url: string
  user: CloudUser | null
  error: string | null
  oidcLoginUrl: string
  capabilities: ServerCapabilities
}

export const CLOUD_ACCOUNT_URL_KEY = 'jaz.cloudAccount.url'
const CLOUD_ACCOUNT_AUTH_PREFIX = 'jaz.cloudAccount.auth.'

const EMPTY_STATE: CloudAccountState = {
  status: 'disconnected',
  url: '',
  user: null,
  error: null,
  oidcLoginUrl: '',
  capabilities: {},
}

const ownsAttachedCloud =
  (clientRuntime.connectionSession?.read()?.owner ?? clientRuntime.windowKind === 'main') &&
  clientRuntime.windowKind !== 'board' &&
  clientRuntime.windowKind !== 'launcher'

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

function authKey(origin: string): string {
  return `${CLOUD_ACCOUNT_AUTH_PREFIX}${normalizeBaseUrl(origin)}`
}

function browserCredKey(origin: string): string {
  return `cloudAccount:${normalizeBaseUrl(origin)}`
}

function readSavedCloudUrl(): string {
  try {
    const raw = localStorage.getItem(CLOUD_ACCOUNT_URL_KEY)?.trim() ?? ''
    return raw ? normalizeBaseUrl(raw) : ''
  } catch {
    return ''
  }
}

function saveCloudUrl(origin: string): void {
  try {
    localStorage.setItem(CLOUD_ACCOUNT_URL_KEY, normalizeBaseUrl(origin))
  } catch {
    // Best-effort persistence.
  }
}

function clearSavedCloudUrl(): void {
  try {
    localStorage.removeItem(CLOUD_ACCOUNT_URL_KEY)
  } catch {
    // ignore
  }
}

function saveEncryptedElectronToken(origin: string, token: string): void {
  if (!token || !clientRuntime.encryptCredential) return
  try {
    const ciphertext = clientRuntime.encryptCredential(token)
    localStorage.setItem(authKey(origin), `enc:v1:${ciphertext}`)
  } catch {
    // Keep the session live in memory even if keychain persistence fails.
  }
}

function restoreElectronToken(origin: string): string {
  let stored = ''
  try {
    stored = localStorage.getItem(authKey(origin))?.trim() ?? ''
  } catch {
    return ''
  }
  if (!stored) return ''
  if (!stored.startsWith('enc:v1:') || !clientRuntime.decryptCredential) {
    throw new Error('Saved Jaz Cloud credential is not securely encrypted')
  }
  return clientRuntime.decryptCredential(stored.slice(7))
}

function clearSavedElectronToken(origin: string): void {
  try {
    localStorage.removeItem(authKey(origin))
  } catch {
    // ignore
  }
}

async function resolveSavedToken(origin: string): Promise<string> {
  if (clientRuntime.kind === 'web') {
    return (await loadBrowserCredential(browserCredKey(origin)))?.token ?? ''
  }
  return restoreElectronToken(origin)
}

function invalidateCloudQueries(): void {
  void queryClient.invalidateQueries({ queryKey: ['cloud'] })
  void queryClient.invalidateQueries({ queryKey: keys.sidebarSessions })
}

function removeCloudQueries(): void {
  queryClient.removeQueries({ queryKey: ['cloud'] })
  void queryClient.invalidateQueries({ queryKey: keys.sidebarSessions })
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

function activateAttached(
  seq: number,
  origin: string,
  session: {
    token: string
    ticket?: ResourceTicket
    user: CloudUser
    oidcLoginUrl: string
    capabilities: ServerCapabilities
    persistCredential: boolean
  },
): void {
  setAttachedCloudSession({
    baseUrl: origin,
    token: session.token,
    ticket: session.ticket,
    user: session.user,
  })
  saveCloudUrl(origin)
  if (session.persistCredential && session.token) {
    if (clientRuntime.kind === 'web') {
      void saveBrowserCredential(browserCredKey(origin), session.token, () => attemptSeq === seq).catch(() => {})
    } else {
      saveEncryptedElectronToken(origin, session.token)
    }
  }
  setState({
    status: 'connected',
    url: origin,
    user: session.user,
    error: null,
    oidcLoginUrl: session.oidcLoginUrl,
    capabilities: session.capabilities,
  })
  invalidateCloudQueries()
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
    const discoveryPromise = fetchServerDiscovery(validated.server, controller.signal)
    if (validated.code) {
      const exchanged = await exchangeOidcCode(
        validated.server,
        validated.code,
        controller.signal,
        validated.codeVerifier,
      )
      if (!isCurrent(seq, controller)) return null
      const discovery = await discoveryPromise
      if (!isCurrent(seq, controller)) return null
      activateAttached(seq, validated.server, {
        token: exchanged.token,
        ticket: exchanged.ticket,
        user: exchanged.user,
        oidcLoginUrl: discovery.ok ? discovery.oidcLoginUrl : '/v1/auth/login',
        capabilities: discovery.ok ? discovery.capabilities : {},
        persistCredential: true,
      })
      return null
    }

    const access = await readCloudAccess(validated.server, validated.token, controller.signal)
    if (!isCurrent(seq, controller)) return null
    const discovery = await discoveryPromise
    if (!isCurrent(seq, controller)) return null
    if (!access.ok || !access.user) {
      const message = access.ok ? 'Cloud server returned no user profile' : access.error
      setState({ status: 'disconnected', error: message })
      return message
    }
    activateAttached(seq, validated.server, {
      token: validated.token,
      ticket: access.ticket,
      user: access.user,
      oidcLoginUrl: discovery.ok ? discovery.oidcLoginUrl : '/v1/auth/login',
      capabilities: discovery.ok ? discovery.capabilities : {},
      persistCredential: true,
    })
    return null
  } catch (error) {
    if (!isCurrent(seq, controller)) return null
    const message = error instanceof Error ? error.message : 'Jaz Cloud sign-in failed'
    setState({ status: 'disconnected', error: message })
    return message
  }
}

export function signOutCloudAccount(): void {
  const prevUrl = state.url || readSavedCloudUrl()
  const target = attachedCloudTarget()
  if (target) {
    void logoutCloudSession(target.baseUrl, target.token)
  }
  activeController?.abort()
  activeController = null
  attemptSeq += 1
  setAttachedCloudSession(null)
  clearCloudThreads()
  clearSavedCloudUrl()
  if (prevUrl) {
    clearSavedElectronToken(prevUrl)
    if (clientRuntime.kind === 'web') {
      void forgetBrowserCredential(browserCredKey(prevUrl)).catch(() => {})
    }
  }
  setState({ ...EMPTY_STATE })
  removeCloudQueries()
}

async function restoreAttachedCloud(): Promise<void> {
  if (!ownsAttachedCloud) return
  if (!clientRuntime.capabilities.localBackend && clientRuntime.kind === 'electron') return
  const savedUrl = readSavedCloudUrl()
  if (!savedUrl || !secureBackendConnection(savedUrl)) return

  const { seq, controller } = beginAttempt()
  setState({ status: 'checking', url: savedUrl, error: null })
  let token = ''
  try {
    token = await resolveSavedToken(savedUrl)
  } catch (error) {
    if (!isCurrent(seq, controller)) return
    setState({
      status: 'disconnected',
      url: savedUrl,
      user: null,
      error: error instanceof Error ? error.message : 'Could not unlock saved Jaz Cloud login.',
    })
    return
  }
  if (!isCurrent(seq, controller)) return
  if (!token && clientRuntime.kind !== 'web') {
    setState({ status: 'disconnected', url: savedUrl, user: null, error: null })
    return
  }

  const [discovery, access] = await Promise.all([
    fetchServerDiscovery(savedUrl, controller.signal),
    readCloudAccess(savedUrl, token, controller.signal),
  ])
  if (!isCurrent(seq, controller)) return
  if (!access.ok || !access.user) {
    setState({
      status: 'disconnected',
      url: savedUrl,
      user: null,
      error: token && !access.ok ? access.error : null,
      oidcLoginUrl: discovery.ok ? discovery.oidcLoginUrl : '/v1/auth/login',
      capabilities: discovery.ok ? discovery.capabilities : {},
    })
    return
  }
  activateAttached(seq, savedUrl, {
    token,
    ticket: access.ticket,
    user: access.user,
    oidcLoginUrl: discovery.ok ? discovery.oidcLoginUrl : '/v1/auth/login',
    capabilities: discovery.ok ? discovery.capabilities : {},
    persistCredential: false,
  })
}

const restorePromise: Promise<void> = restoreAttachedCloud().catch(() => {
  setState({ status: 'disconnected' })
})

export function waitForCloudRestore(): Promise<void> {
  return restorePromise
}

onCloudAuthRejected(() => {
  const prevUrl = state.url
  activeController?.abort()
  activeController = null
  attemptSeq += 1
  setAttachedCloudSession(null)
  clearCloudThreads()
  setState({
    status: 'disconnected',
    url: prevUrl,
    user: null,
    error: 'Cloud session expired. Sign in again.',
  })
  removeCloudQueries()
})
