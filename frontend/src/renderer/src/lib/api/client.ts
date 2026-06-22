const BACKEND_URL_KEY = 'jaz.backendUrl'
const AUTH_KEY_PREFIX = 'jaz.backendAuth.'
const DEFAULT_LOCAL_URL = 'http://localhost:5299'
export const CLIENT_PLATFORM_HEADER = 'X-Jaz-Client-Platform'
export const CLIENT_PLATFORM = 'desktop'

// The local default bridged by the preload script; fall back for
// plain-browser debugging.
export function localBaseUrl(): string {
  return window.jaz?.apiBaseUrl ?? DEFAULT_LOCAL_URL
}

export function normalizeBaseUrl(url: string): string {
  let trimmed = url.trim().replace(/\/+$/, '')
  if (trimmed && !/^https?:\/\//i.test(trimmed)) trimmed = `http://${trimmed}`
  // keep only the origin: a pasted path like /health would break every request
  try {
    return new URL(trimmed).origin
  } catch {
    return trimmed
  }
}

export function parseBackendConnectUrl(input: string): { url: string; key: string } {
  let raw = input.trim()
  if (raw && !/^https?:\/\//i.test(raw)) raw = `http://${raw}`
  try {
    const parsed = new URL(raw)
    const key = parsed.searchParams.get('key')?.trim() ?? ''
    parsed.searchParams.delete('key')
    parsed.pathname = ''
    parsed.hash = ''
    parsed.search = ''
    return { url: parsed.origin, key }
  } catch {
    return { url: normalizeBaseUrl(input), key: '' }
  }
}

// A remembered remote URL wins over the local default so the next launch
// reconnects to wherever the user pointed the app last — unless JAZ_API_URL
// was set explicitly (≠ default), which is a developer override that beats
// the remembered URL.
let baseUrl = ((): string => {
  const local = localBaseUrl()
  if (local !== DEFAULT_LOCAL_URL) return normalizeBaseUrl(local)
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

export function apiAuthenticatedWebSocketUrl(path: string): string {
  return appendAuthQuery(apiWebSocketUrl(path))
}

export function setApiBaseUrl(url: string): void {
  baseUrl = normalizeBaseUrl(url)
  localStorage.setItem(BACKEND_URL_KEY, baseUrl)
}

function authStorageKey(url: string): string {
  return `${AUTH_KEY_PREFIX}${normalizeBaseUrl(url)}`
}

export function apiAuthToken(url = apiBaseUrl()): string {
  return localStorage.getItem(authStorageKey(url))?.trim() ?? ''
}

export function setApiAuthToken(url: string, token?: string | null): void {
  const key = authStorageKey(url)
  const value = token?.trim() ?? ''
  if (value) localStorage.setItem(key, value)
  else localStorage.removeItem(key)
}

function withAPIHeaders(init: RequestInit = {}, url = apiBaseUrl()): RequestInit {
  const token = apiAuthToken(url)
  const headers = new Headers(init.headers)
  if (!headers.has(CLIENT_PLATFORM_HEADER)) headers.set(CLIENT_PLATFORM_HEADER, CLIENT_PLATFORM)
  if (token && !headers.has('Authorization')) headers.set('Authorization', `Bearer ${token}`)
  return { ...init, headers }
}

function appendAuthQuery(rawUrl: string, url = apiBaseUrl()): string {
  const token = apiAuthToken(url)
  if (!token) return rawUrl
  try {
    const parsed = new URL(rawUrl)
    parsed.searchParams.set('key', token)
    return parsed.toString()
  } catch {
    return `${rawUrl}${rawUrl.includes('?') ? '&' : '?'}key=${encodeURIComponent(token)}`
  }
}

export function apiFetch(path: string, init?: RequestInit): Promise<Response> {
  assertBackendRelativePath(path, 'apiFetch')
  return fetch(apiUrl(path), withAPIHeaders(init))
}

export function apiEventSourceUrl(path: string): string {
  return appendAuthQuery(apiUrl(path))
}

export class ApiError extends Error {
  status: number

  constructor(status: number, message: string) {
    super(message)
    this.status = status
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await apiFetch(path, init)
  if (!res.ok) {
    let message = `${res.status} ${res.statusText}`
    try {
      const body = (await res.json()) as { error?: string }
      if (body.error) message = body.error
    } catch {
      // non-JSON error body; keep the status text
    }
    throw new ApiError(res.status, message)
  }
  return (await res.json()) as T
}

export function get<T>(path: string): Promise<T> {
  return request<T>(path)
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

export function post<T>(path: string, body?: unknown): Promise<T> {
  return request<T>(path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
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
