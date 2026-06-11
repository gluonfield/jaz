const BACKEND_URL_KEY = 'jaz.backendUrl'
const DEFAULT_LOCAL_URL = 'http://localhost:5299'

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

// A remembered remote URL wins over the local default so the next launch
// reconnects to wherever the user pointed the app last — unless JAZ_API_URL
// was set explicitly (≠ default), which is a developer override that beats
// the remembered URL.
// Local URLs remembered before the default port moved to 5299; a stored
// value pointing at the old local default should not pin the app to it.
const LEGACY_LOCAL_URLS = new Set(['http://localhost:8080', 'http://127.0.0.1:8080'])

let baseUrl = ((): string => {
  const local = localBaseUrl()
  if (local !== DEFAULT_LOCAL_URL) return normalizeBaseUrl(local)
  const stored = localStorage.getItem(BACKEND_URL_KEY)
  if (stored && LEGACY_LOCAL_URLS.has(normalizeBaseUrl(stored))) {
    localStorage.removeItem(BACKEND_URL_KEY)
    return local
  }
  return stored ? normalizeBaseUrl(stored) : local
})()

export function apiBaseUrl(): string {
  return baseUrl
}

export function setApiBaseUrl(url: string): void {
  baseUrl = normalizeBaseUrl(url)
  localStorage.setItem(BACKEND_URL_KEY, baseUrl)
}

export class ApiError extends Error {
  status: number

  constructor(status: number, message: string) {
    super(message)
    this.status = status
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${apiBaseUrl()}${path}`, init)
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
