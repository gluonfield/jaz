import { readAPIResponse } from '@/lib/api/response'
import { DEFAULT_API_BASE_URL, clientRuntime } from '@/lib/clientRuntime'
import { isPreviewProxyTargetHostname } from '@shared/preview'

export { ApiError } from '@/lib/api/response'

const BACKEND_URL_KEY = 'jaz.backendUrl'
const AUTH_KEY_PREFIX = 'jaz.backendAuth.'
const authTokens = new Map<string, string>()
const resourceTickets = new Map<string, { token: string; expires_at: string }>()
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

export function parseBackendConnectUrl(input: string): { url: string; key: string } {
  let raw = input.trim()
  if (raw && !/^https?:\/\//i.test(raw)) {
    raw = defaultConnectionScheme(raw)
  }
  try {
    const parsed = new URL(raw)
    const key = (parsed.searchParams.get('key') || new URLSearchParams(parsed.hash.slice(1)).get('key'))?.trim() ?? ''
    parsed.searchParams.delete('key')
    parsed.pathname = ''
    parsed.hash = ''
    parsed.search = ''
    return { url: parsed.origin, key }
  } catch {
    return { url: normalizeBaseUrl(input), key: '' }
  }
}

export function consumeStartupConnectUrl(): string {
  if (clientRuntime.kind !== 'web') return ''
  const { params, source } = startupConnectParams()
  const key = params.get('key')?.trim() ?? ''
  if (!key) return ''
  const target =
    params.get('server')?.trim() ||
    params.get('url')?.trim() ||
    clientRuntime.defaultApiBaseUrl()
  if (!target) return ''
  const raw = connectUrlWithKey(target, key)
  const next = new URL(window.location.href)
  if (source === 'hash') {
    next.hash = ''
  } else {
    next.searchParams.delete('server')
    next.searchParams.delete('url')
    next.searchParams.delete('key')
  }
  window.history.replaceState(window.history.state, '', next)
  return raw
}

function startupConnectParams(): { params: URLSearchParams; source: 'search' | 'hash' } {
  const search = new URLSearchParams(window.location.search)
  if (hasStartupConnectParams(search)) return { params: search, source: 'search' }
  const hash = new URLSearchParams(window.location.hash.replace(/^#\??/, ''))
  return { params: hash, source: 'hash' }
}

function hasStartupConnectParams(params: URLSearchParams): boolean {
  return Boolean((params.get('server')?.trim() || params.get('url')?.trim()) && params.get('key')?.trim())
}

function connectUrlWithKey(target: string, key: string): string {
  const raw = target.trim()
  try {
    const parsed = new URL(/^https?:\/\//i.test(raw) ? raw : defaultConnectionScheme(raw))
    parsed.hash = new URLSearchParams({ key }).toString()
    return parsed.toString()
  } catch {
    return `${raw}#key=${encodeURIComponent(key)}`
  }
}

// A remembered remote URL wins over the local default so refresh reconnects to
// the user's server. An explicit runtime default still wins for desktop preload
// and VITE_JAZ_API_URL-pinned development builds.
let baseUrl = ((): string => {
  const local = localBaseUrl()
  if (local !== DEFAULT_API_BASE_URL) return normalizeBaseUrl(local)
  const stored = localStorage.getItem(BACKEND_URL_KEY)
  return stored ? normalizeBaseUrl(stored) : local
})()

export function apiBaseUrl(): string {
  return baseUrl
}

export function apiUrl(path: string): string {
  return `${apiBaseUrl()}${path}`
}

export function apiWebSocketUrl(path: string): string {
  assertBackendRelativePath(path, 'apiWebSocketUrl')
  const url = new URL(path, `${apiBaseUrl()}/`)
  url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:'
  return url.toString()
}

export function apiWebSocketProtocols(): string[] {
  if (!secureBackendConnection(apiBaseUrl())) {
    throw new Error('Remote connections require HTTPS')
  }
  const token = apiAuthToken()
  return token ? [`jaz-auth.${token}`] : []
}

export function apiWebSocket(path: string): WebSocket {
  return new WebSocket(apiWebSocketUrl(path), apiWebSocketProtocols())
}

export function apiEmbeddedGetUrl(path: string): string {
  assertBackendRelativePath(path, 'apiEmbeddedGetUrl')
  return appendAuthQuery(apiUrl(path))
}

export function setApiBaseUrl(url: string): void {
  baseUrl = normalizeBaseUrl(url)
  localStorage.setItem(BACKEND_URL_KEY, baseUrl)
}

function authStorageKey(url: string): string {
  return `${AUTH_KEY_PREFIX}${normalizeBaseUrl(url)}`
}

export function apiAuthToken(url = apiBaseUrl()): string {
  const key = authStorageKey(url)
  const cached = authTokens.get(key)
  if (cached !== undefined) {
    return cached
  }
  const stored = localStorage.getItem(key)?.trim() ?? ''
  if (!stored) {
    return ''
  }
  const token = stored.startsWith('enc:v1:')
    ? clientRuntime.decryptCredential?.(stored.slice(7)) ?? ''
    : stored
  if (clientRuntime.encryptCredential && !stored.startsWith('enc:v1:')) {
    localStorage.setItem(key, 'enc:v1:' + clientRuntime.encryptCredential(token))
  }
  authTokens.set(key, token)
  return token
}

export function setApiAuthToken(url: string, token?: string | null): void {
  const key = authStorageKey(url)
  const value = token?.trim() ?? ''
  resourceTickets.delete(normalizeBaseUrl(url))
  if (value) {
    const stored = clientRuntime.encryptCredential ? 'enc:v1:' + clientRuntime.encryptCredential(value) : value
    localStorage.setItem(key, stored)
    authTokens.set(key, value)
  } else {
    localStorage.removeItem(key)
    authTokens.delete(key)
  }
}

function withAPIHeaders(init: RequestInit = {}, url = apiBaseUrl()): RequestInit {
  if (!secureBackendConnection(url)) {
    throw new Error('Remote connections require HTTPS')
  }
  const token = apiAuthToken(url)
  const headers = new Headers(init.headers)
  if (!headers.has(CLIENT_PLATFORM_HEADER)) headers.set(CLIENT_PLATFORM_HEADER, CLIENT_PLATFORM)
  if (token && !headers.has('Authorization')) headers.set('Authorization', `Bearer ${token}`)
  return { ...init, headers }
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

export function apiFetch(path: string, init?: RequestInit): Promise<Response> {
  assertBackendRelativePath(path, 'apiFetch')
  return fetch(apiUrl(path), withAPIHeaders(init))
}

export function apiEventSourceUrl(path: string): string {
  return appendAuthQuery(apiUrl(path))
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

function assertBackendRelativePath(path: string, helper: string): void {
  if (/^(?:https?|wss?):\/\//i.test(path)) {
    throw new Error(`${helper} only accepts backend-relative paths`)
  }
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
