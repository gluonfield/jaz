import { readAPIResponse } from '@/lib/api/response'
import { DEFAULT_API_BASE_URL, backendFetch, backendTransportUrl, clientRuntime } from '@/lib/clientRuntime'
import { decodeStoredCredential, encodeStoredCredential } from '@/lib/credentialEnvelope'
import { isPreviewProxyTargetHostname } from '@shared/preview'
import type { ResourceTicket } from '@shared/connectionSession'
import type { ServerMode } from './serverTypes'

export { ApiError } from '@/lib/api/response'

export type SessionRefreshOutcome = 'refreshed' | 'unauthorized' | 'error' | 'unavailable'
export type SessionRefresher = (params: {
  origin: string
  mode: 'cookie' | 'bearer'
  refreshToken: string
  signal: AbortSignal
}) => Promise<SessionRefreshOutcome>

const AUTH_KEY_PREFIX = 'jaz.backendAuth.'
const authTokens = new Map<string, string>()
const refreshTokens = new Map<string, string>()
const refreshGenerations = new Map<string, number>()
const resourceTickets = new Map<string, ResourceTicket>()
let serverMode: ServerMode = 'local'
let credentialRevision = 0
const sessionListeners = new Set<() => void>()
let sessionRequests = new AbortController()
const rejectionListeners = new Set<() => void>()
let sessionRefresher: SessionRefresher | null = null
let inFlightRefresh: { origin: string; revision: number; promise: Promise<SessionRefreshOutcome> } | null = null
// The port the local backend always runs on; a loopback host on this port is
// "this machine", any other loopback port is a tunnel to a remote backend.
export const DEFAULT_LOCAL_PORT = new URL(DEFAULT_API_BASE_URL).port
export const CLIENT_PLATFORM_HEADER = 'X-Jaz-Client-Platform'
export const CLIENT_PLATFORM = clientRuntime.platform

// The local default bridged by the preload script; fall back for
// plain-browser debugging.
export function localBaseUrl(): string {
  return clientRuntime.defaultApiBaseUrl()
}

export function normalizeBaseUrl(url: string): string {
  let trimmed = url.trim().replace(/\/+$/, '')
  if (trimmed && !/^https?:\/\//i.test(trimmed)) {
    trimmed = defaultConnectionScheme(trimmed)
  }
  // keep only the origin: a pasted path like /health would break every request
  try {
    return new URL(trimmed).origin
  } catch {
    return trimmed
  }
}

export function isLocalBackendUrl(url: string): boolean {
  try {
    const { hostname, port } = new URL(normalizeBaseUrl(url))
    return isPreviewProxyTargetHostname(hostname) && port === DEFAULT_LOCAL_PORT
  } catch {
    return false
  }
}

// Parses a manually entered backend connect URL. Secrets are read strictly from
// the URL fragment (#key= or #token=) and never from query parameters (?key=/?token=).
export function parseBackendConnectUrl(input: string): { url: string; key: string } {
  let raw = input.trim()
  if (raw && !/^https?:\/\//i.test(raw)) {
    raw = defaultConnectionScheme(raw)
  }
  try {
    const parsed = new URL(raw)
    const hashParams = new URLSearchParams(parsed.hash.replace(/^#\??/, ''))
    const key = (hashParams.get('key') || hashParams.get('token'))?.trim() ?? ''
    parsed.pathname = ''
    parsed.hash = ''
    parsed.search = ''
    return { url: parsed.origin, key }
  } catch {
    return { url: normalizeBaseUrl(input), key: '' }
  }
}

let baseUrl = normalizeBaseUrl(localBaseUrl())

export function apiBaseUrl(): string {
  return baseUrl
}

export function apiUrl(path: string): string {
  return resolveBackendUrl(path, apiBaseUrl(), 'apiUrl')
}

export function apiWebSocketUrl(path: string): string {
  const url = new URL(resolveBackendUrl(path, apiBaseUrl(), 'apiWebSocketUrl'))
  url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:'
  return url.toString()
}

export function apiWebSocketProtocols(): string[] {
  if (!secureBackendConnection(apiBaseUrl())) {
    throw new Error('Remote connections require HTTPS')
  }
  const token = cachedApiAuthToken()
  return token ? ['jaz', `jaz-auth.${token}`] : []
}

export function apiWebSocket(path: string): WebSocket {
  const socket = new WebSocket(apiWebSocketUrl(path), apiWebSocketProtocols())
  const unsubscribe = onApiSessionChange(() => socket.close())
  socket.addEventListener('close', unsubscribe, { once: true })
  return socket
}

export function apiEmbeddedGetUrl(path: string): string {
  return backendTransportUrl(appendAuthQuery(resolveBackendUrl(path, apiBaseUrl(), 'apiEmbeddedGetUrl')))
}

export function setApiBaseUrl(url: string): void {
  if (baseUrl === normalizeBaseUrl(url)) {
    return
  }
  baseUrl = normalizeBaseUrl(url)
  notifySessionChange()
}

function authStorageKey(url: string): string {
  return `${AUTH_KEY_PREFIX}${normalizeBaseUrl(url)}`
}

export function cachedApiAuthToken(url = apiBaseUrl()): string {
  return authTokens.get(authStorageKey(url)) ?? ''
}

export function cachedApiRefreshToken(url = apiBaseUrl()): string {
  return refreshTokens.get(authStorageKey(url)) ?? ''
}

export function apiRefreshGeneration(url = apiBaseUrl()): number {
  return refreshGenerations.get(normalizeBaseUrl(url)) ?? 0
}

export function restoreApiCredential(url: string): { token: string; refreshToken?: string } {
  const stored = localStorage.getItem(authStorageKey(url))?.trim() ?? ''
  if (!stored) {
    return { token: '' }
  }
  if (!stored.startsWith('enc:v1:') || !clientRuntime.decryptCredential) {
    throw new Error('Saved connection is not securely encrypted')
  }
  return decodeStoredCredential(clientRuntime.decryptCredential(stored.slice(7)))
}

export function restoreApiAuthToken(url: string): string {
  return restoreApiCredential(url).token
}

export function rememberApiAuthToken(url: string, current: () => boolean): void {
  const token = cachedApiAuthToken(url)
  const refreshToken = cachedApiRefreshToken(url)
  const revision = credentialRevision
  if (!token || !clientRuntime.encryptCredential || !current()) {
    throw new Error('Secure credential storage is unavailable')
  }
  const ciphertext = clientRuntime.encryptCredential(encodeStoredCredential(token, refreshToken))
  if (revision !== credentialRevision || token !== cachedApiAuthToken(url) || refreshToken !== cachedApiRefreshToken(url) || !current()) {
    throw new Error('Connection changed before it could be remembered')
  }
  localStorage.setItem(authStorageKey(url), 'enc:v1:' + ciphertext)
}

export function setApiAuthToken(
  url: string,
  token?: string | null,
  ticket?: ResourceTicket,
  refreshToken?: string | null,
): void {
  const key = authStorageKey(url)
  const origin = normalizeBaseUrl(url)
  if (token) {
    authTokens.set(key, token)
    if (refreshToken) {
      refreshTokens.set(key, refreshToken)
    } else if (refreshToken === null || refreshToken === '') {
      refreshTokens.delete(key)
    }
  } else {
    authTokens.delete(key)
    refreshTokens.delete(key)
  }
  refreshGenerations.delete(origin)
  setResourceTicket(url, ticket)
  if (origin === baseUrl) {
    notifySessionChange()
  }
}

export function rotateApiSessionTokens(
  url: string,
  update: { token?: string; refreshToken?: string; ticket?: ResourceTicket },
): void {
  const key = authStorageKey(url)
  const origin = normalizeBaseUrl(url)
  if (update.token) {
    authTokens.set(key, update.token)
  }
  if (update.refreshToken !== undefined) {
    if (update.refreshToken) {
      refreshTokens.set(key, update.refreshToken)
    } else {
      refreshTokens.delete(key)
    }
  }
  if (update.ticket) {
    setResourceTicket(origin, update.ticket)
  }
  refreshGenerations.set(origin, (refreshGenerations.get(origin) ?? 0) + 1)
}

export function apiServerMode(): ServerMode {
  return serverMode
}

export function isCloudApiSession(): boolean {
  return serverMode === 'cloud'
}

export function setApiServerMode(mode: ServerMode): void {
  serverMode = mode
}

export function clearApiSession(): void {
  authTokens.clear()
  refreshTokens.clear()
  refreshGenerations.clear()
  resourceTickets.clear()
  inFlightRefresh = null
  serverMode = 'local'
  notifySessionChange()
}

export function forgetApiAuthToken(url: string): void {
  setApiAuthToken(url, null)
  localStorage.removeItem(authStorageKey(url))
}

export function apiSessionRevision(): number {
  return credentialRevision
}

export function onApiSessionChange(handler: () => void): () => void {
  sessionListeners.add(handler)
  return () => sessionListeners.delete(handler)
}

export function onApiAuthRejected(handler: () => void): () => void {
  rejectionListeners.add(handler)
  return () => rejectionListeners.delete(handler)
}

export function setApiSessionRefresher(handler: SessionRefresher | null): void {
  sessionRefresher = handler
}

function notifyAuthRejected(revision: number, origin: string): void {
  if (revision !== credentialRevision || origin !== apiBaseUrl()) {
    return
  }
  for (const handler of rejectionListeners) {
    handler()
  }
}

export function refreshActiveApiSession(
  origin: string,
  sentToken: string,
  sentRefreshGen: number,
  cookieAuthed: boolean,
): Promise<SessionRefreshOutcome> {
  const normalized = normalizeBaseUrl(origin)
  const revision = credentialRevision
  if (cookieAuthed) {
    if (apiRefreshGeneration(normalized) > sentRefreshGen) {
      return Promise.resolve('refreshed')
    }
  } else {
    const currentToken = cachedApiAuthToken(normalized)
    if (currentToken && currentToken !== sentToken) {
      return Promise.resolve('refreshed')
    }
  }
  if (!sessionRefresher) {
    return Promise.resolve('unavailable')
  }
  const refreshToken = cookieAuthed ? '' : cachedApiRefreshToken(normalized)
  if (!cookieAuthed && !refreshToken) {
    return Promise.resolve('unavailable')
  }
  if (inFlightRefresh && inFlightRefresh.origin === normalized && inFlightRefresh.revision === revision) {
    return inFlightRefresh.promise
  }
  const refresher = sessionRefresher
  const flight: { origin: string; revision: number; promise: Promise<SessionRefreshOutcome> } = {
    origin: normalized,
    revision,
    promise: Promise.resolve('unavailable'),
  }
  flight.promise = Promise.resolve()
    .then(() =>
      refresher({
        origin: normalized,
        mode: cookieAuthed ? 'cookie' : 'bearer',
        refreshToken,
        signal: sessionRequests.signal,
      }),
    )
    .catch(() => 'error' as const)
    .finally(() => {
      if (inFlightRefresh === flight) {
        inFlightRefresh = null
      }
    })
  inFlightRefresh = flight
  return flight.promise
}

function notifySessionChange(): void {
  credentialRevision += 1
  inFlightRefresh = null
  sessionRequests.abort()
  sessionRequests = new AbortController()
  for (const handler of sessionListeners) {
    handler()
  }
}

function withAPIHeaders(init: RequestInit = {}, url = apiBaseUrl()): RequestInit {
  if (!secureBackendConnection(url)) {
    throw new Error('Remote connections require HTTPS')
  }
  const token = cachedApiAuthToken(url)
  const headers = new Headers(init.headers)
  if (!headers.has(CLIENT_PLATFORM_HEADER)) {
    headers.set(CLIENT_PLATFORM_HEADER, CLIENT_PLATFORM)
  }
  if (token && !headers.has('Authorization')) {
    headers.set('Authorization', `Bearer ${token}`)
  }
  const credentials =
    init.credentials ??
    (!token && clientRuntime.kind === 'web' && isCloudApiSession() ? 'include' : undefined)
  return credentials === undefined ? { ...init, headers } : { ...init, headers, credentials }
}

function appendAuthQuery(rawUrl: string, url = apiBaseUrl()): string {
  if (!secureBackendConnection(url)) {
    throw new Error('Remote connections require HTTPS')
  }
  const token = resourceTickets.get(normalizeBaseUrl(url))?.token
  if (!token) {
    return rawUrl
  }
  const parsed = new URL(rawUrl)
  parsed.searchParams.set('ticket', token)
  return parsed.toString()
}

function isAuthLifecyclePath(targetUrl: string): boolean {
  try {
    const pathname = new URL(targetUrl).pathname
    return pathname === '/v1/auth/refresh' || pathname === '/v1/auth/logout' || pathname === '/v1/auth/exchange'
  } catch {
    return false
  }
}

function isReplayableBody(body: BodyInit | null | undefined): boolean {
  return !(typeof ReadableStream !== 'undefined' && body instanceof ReadableStream)
}

export function apiFetch(path: string, init?: RequestInit, url = apiBaseUrl()): Promise<Response> {
  const targetUrl = resolveBackendUrl(path, url, 'apiFetch')
  const origin = normalizeBaseUrl(url)
  const signal = init?.signal ? AbortSignal.any([init.signal, sessionRequests.signal]) : sessionRequests.signal
  const revision = credentialRevision
  const sentToken = cachedApiAuthToken(origin)
  const sentRefreshGen = apiRefreshGeneration(origin)
  const callerSuppliedAuth = new Headers(init?.headers).has('Authorization')
  const options = withAPIHeaders({ ...init, signal }, origin)
  const usedSessionBearer = !callerSuppliedAuth && Boolean(sentToken)
  const usedSessionCookie = !callerSuppliedAuth && !sentToken && options.credentials === 'include'
  const usedSessionAuth = usedSessionBearer || usedSessionCookie

  return backendFetch(targetUrl, options).then(async (response) => {
    if ((response.status !== 401 && response.status !== 403) || !usedSessionAuth) {
      return response
    }
    const body = await response.clone().json().catch(() => null) as { code?: string } | null
    if (revision !== credentialRevision || origin !== apiBaseUrl() || body?.code !== 'unauthorized') {
      return response
    }
    if (response.status === 401 && isCloudApiSession() && !isAuthLifecyclePath(targetUrl)) {
      const outcome = await refreshActiveApiSession(origin, sentToken, sentRefreshGen, usedSessionCookie)
      if (outcome === 'refreshed') {
        if (revision !== credentialRevision || origin !== apiBaseUrl() || init?.signal?.aborted) {
          return response
        }
        if (!isReplayableBody(init?.body)) {
          return response
        }
        const retrySignal = init?.signal ? AbortSignal.any([init.signal, sessionRequests.signal]) : sessionRequests.signal
        const retryOptions = withAPIHeaders({ ...init, signal: retrySignal }, origin)
        const retryResponse = await backendFetch(targetUrl, retryOptions)
        if (retryResponse.status === 401) {
          const retryBody = await retryResponse.clone().json().catch(() => null) as { code?: string } | null
          if (retryBody?.code === 'unauthorized') {
            notifyAuthRejected(revision, origin)
          }
        }
        return retryResponse
      }
      if (outcome === 'error') {
        return response
      }
    }
    notifyAuthRejected(revision, origin)
    return response
  })
}

export function apiEventSourceUrl(path: string): string {
  return backendTransportUrl(appendAuthQuery(resolveBackendUrl(path, apiBaseUrl(), 'apiEventSourceUrl')))
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  return readAPIResponse<T>(await apiFetch(path, init))
}

export function get<T>(path: string, init?: RequestInit): Promise<T> {
  return request<T>(path, init)
}

export function put<T>(path: string, body: unknown): Promise<T> {
  return request<T>(path, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
}

export function patch<T>(path: string, body: unknown): Promise<T> {
  return request<T>(path, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
}

export function post<T>(path: string, body?: unknown, signal?: AbortSignal): Promise<T> {
  return request<T>(path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
    signal,
  })
}

export function del<T>(path: string): Promise<T> {
  return request<T>(path, { method: 'DELETE' })
}

export function assertBackendRelativePath(path: string, helper: string): void {
  if (!path.startsWith('/') || path.startsWith('//') || path.startsWith('/\\') || path.includes('\\')) {
    throw new Error(`${helper} only accepts backend-relative paths`)
  }
}

export function resolveBackendUrl(path: string, base: string, helper: string): string {
  assertBackendRelativePath(path, helper)
  const origin = normalizeBaseUrl(base)
  let resolved: URL
  try {
    resolved = new URL(path, `${origin}/`)
  } catch {
    throw new Error(`${helper} received an invalid path`)
  }
  if (resolved.origin !== origin) {
    throw new Error(`${helper} only accepts backend-relative paths`)
  }
  return resolved.toString()
}

export function cachedResourceTicket(url = apiBaseUrl()): ResourceTicket | undefined {
  return resourceTickets.get(normalizeBaseUrl(url))
}

export function setResourceTicket(url: string, ticket?: { token: string; expires_at: string }): void {
  if (ticket) {
    resourceTickets.set(normalizeBaseUrl(url), ticket)
  } else {
    resourceTickets.delete(normalizeBaseUrl(url))
  }
}

function defaultConnectionScheme(value: string): string {
  const host = value.split(/[/:?#]/)[0]?.toLowerCase()
  const local = host === 'localhost' || host === '127.0.0.1' || value.startsWith('[::1]')
  return `${local ? 'http' : 'https'}://${value}`
}

export function secureBackendConnection(url: string): boolean {
  try {
    const parsed = new URL(url)
    return parsed.protocol === 'https:' || (parsed.protocol === 'http:' && ['localhost', '127.0.0.1', '[::1]'].includes(parsed.hostname))
  } catch {
    return false
  }
}
