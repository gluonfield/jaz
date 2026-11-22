import { readAPIResponse } from '@/lib/api/response'
import { backendFetch, backendTransportUrl, clientRuntime } from '@/lib/clientRuntime'
import type { ResourceTicket } from '@shared/connectionSession'
import {
  apiBaseUrl,
  apiEmbeddedGetUrl,
  apiFetch,
  apiSessionRevision,
  cachedApiAuthToken,
  cachedResourceTicket,
  CLIENT_PLATFORM,
  CLIENT_PLATFORM_HEADER,
  isCloudApiSession,
  normalizeBaseUrl,
  onApiSessionChange,
  resolveBackendUrl,
  secureBackendConnection,
} from './client'
import type { CloudUser } from './serverTypes'

export interface CloudTarget {
  baseUrl: string
  token: string
  ticket?: ResourceTicket
  user: CloudUser | null
}

const CLOUD_THREADS_STORAGE_KEY = 'jaz.cloudThreadIds'
const MAX_STORED_CLOUD_THREADS = 500

let attachedTarget: CloudTarget | null = null
let primaryCloudUser: CloudUser | null = null
let attachedRevision = 0
let attachedRequests = new AbortController()
const attachedSessionListeners = new Set<() => void>()
const attachedRejectionListeners = new Set<() => void>()

let loadedThreadIds = false
const cloudThreadIds = new Set<string>()

function ensureCloudThreadsLoaded(): void {
  if (loadedThreadIds) return
  loadedThreadIds = true
  try {
    const raw = sessionStorage.getItem(CLOUD_THREADS_STORAGE_KEY)
    if (!raw) return
    const parsed: unknown = JSON.parse(raw)
    if (Array.isArray(parsed)) {
      for (const item of parsed) {
        if (typeof item === 'string' && item) cloudThreadIds.add(item)
      }
    }
  } catch {
    // sessionStorage may be unavailable in restricted contexts.
  }
}

function persistCloudThreads(): void {
  try {
    const entries = [...cloudThreadIds]
    const trimmed = entries.length > MAX_STORED_CLOUD_THREADS
      ? entries.slice(entries.length - MAX_STORED_CLOUD_THREADS)
      : entries
    sessionStorage.setItem(CLOUD_THREADS_STORAGE_KEY, JSON.stringify(trimmed))
  } catch {
    // Best-effort cache; in-memory set remains authoritative for this tab.
  }
}

export function registerCloudThread(id: string): void {
  if (!id) return
  ensureCloudThreadsLoaded()
  if (cloudThreadIds.has(id)) return
  cloudThreadIds.add(id)
  persistCloudThreads()
}

export function registerCloudThreads(ids: Iterable<string>): void {
  ensureCloudThreadsLoaded()
  let changed = false
  for (const id of ids) {
    if (id && !cloudThreadIds.has(id)) {
      cloudThreadIds.add(id)
      changed = true
    }
  }
  if (changed) persistCloudThreads()
}

export function clearCloudThreads(): void {
  loadedThreadIds = true
  cloudThreadIds.clear()
  try {
    sessionStorage.removeItem(CLOUD_THREADS_STORAGE_KEY)
  } catch {
    // ignore
  }
}

export function isCloudThread(sessionId: string): boolean {
  if (isCloudApiSession()) return true
  if (!sessionId) return false
  ensureCloudThreadsLoaded()
  return cloudThreadIds.has(sessionId)
}

function notifyAttachedSessionChange(): void {
  attachedRevision += 1
  attachedRequests.abort()
  attachedRequests = new AbortController()
  for (const handler of attachedSessionListeners) {
    handler()
  }
}

export function setAttachedCloudSession(target: CloudTarget | null): void {
  attachedTarget = target
    ? {
        baseUrl: normalizeBaseUrl(target.baseUrl),
        token: target.token,
        ticket: target.ticket,
        user: target.user,
      }
    : null
  notifyAttachedSessionChange()
}

export function attachedCloudTarget(): CloudTarget | null {
  return attachedTarget
}

export function setPrimaryCloudUser(user: CloudUser | null): void {
  primaryCloudUser = user
}

export function activeCloudTarget(): CloudTarget | null {
  if (isCloudApiSession()) {
    return {
      baseUrl: apiBaseUrl(),
      token: cachedApiAuthToken(),
      ticket: cachedResourceTicket(),
      user: primaryCloudUser,
    }
  }
  return attachedTarget
}

export function hasActiveCloudTarget(): boolean {
  return activeCloudTarget() !== null
}

export function cloudSessionRevision(): string {
  return `${apiSessionRevision()}:${attachedRevision}`
}

export function onCloudSessionChange(handler: () => void): () => void {
  const unsubPrimary = onApiSessionChange(handler)
  attachedSessionListeners.add(handler)
  return () => {
    unsubPrimary()
    attachedSessionListeners.delete(handler)
  }
}

export function onCloudAuthRejected(handler: () => void): () => void {
  attachedRejectionListeners.add(handler)
  return () => attachedRejectionListeners.delete(handler)
}

export function cloudFetch(path: string, init?: RequestInit): Promise<Response> {
  if (isCloudApiSession()) {
    return apiFetch(path, init)
  }
  const target = attachedTarget
  if (!target) {
    return Promise.reject(new Error('Not signed in to Jaz Cloud'))
  }
  if (!secureBackendConnection(target.baseUrl)) {
    return Promise.reject(new Error('Remote connections require HTTPS'))
  }
  const url = resolveBackendUrl(path, target.baseUrl, 'cloudFetch')
  const signal = init?.signal
    ? AbortSignal.any([init.signal, attachedRequests.signal])
    : attachedRequests.signal
  const headers = new Headers(init?.headers)
  if (!headers.has(CLIENT_PLATFORM_HEADER)) {
    headers.set(CLIENT_PLATFORM_HEADER, CLIENT_PLATFORM)
  }
  if (target.token && !headers.has('Authorization')) {
    headers.set('Authorization', `Bearer ${target.token}`)
  }
  const revision = attachedRevision
  const bearer = headers.get('Authorization')
  const credentials = init?.credentials ?? (!target.token && clientRuntime.kind === 'web' ? 'include' : undefined)

  return backendFetch(url, { ...init, headers, credentials, signal }).then(async (response) => {
    if (response.status === 401 || response.status === 403) {
      const sameCred = target.token
        ? bearer === `Bearer ${attachedTarget?.token ?? ''}`
        : !attachedTarget?.token
      if (sameCred && revision === attachedRevision) {
        const body = await response.clone().json().catch(() => null) as { code?: string } | null
        if (response.status === 401 || body?.code === 'unauthorized') {
          for (const handler of attachedRejectionListeners) {
            handler()
          }
        }
      }
    }
    return response
  })
}

export function cloudEmbeddedGetUrl(path: string): string {
  if (isCloudApiSession()) {
    return apiEmbeddedGetUrl(path)
  }
  const target = attachedTarget
  if (!target) {
    throw new Error('Not signed in to Jaz Cloud')
  }
  if (!secureBackendConnection(target.baseUrl)) {
    throw new Error('Remote connections require HTTPS')
  }
  const resolved = resolveBackendUrl(path, target.baseUrl, 'cloudEmbeddedGetUrl')
  const ticket = target.ticket?.token
  if (!ticket) {
    return backendTransportUrl(resolved)
  }
  const parsed = new URL(resolved)
  parsed.searchParams.set('ticket', ticket)
  return backendTransportUrl(parsed.toString())
}

async function cloudRequest<T>(path: string, init?: RequestInit): Promise<T> {
  return readAPIResponse<T>(await cloudFetch(path, init))
}

export function cloudGet<T>(path: string, init?: RequestInit): Promise<T> {
  return cloudRequest<T>(path, init)
}

export function cloudPost<T>(path: string, body?: unknown, signal?: AbortSignal): Promise<T> {
  return cloudRequest<T>(path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
    signal,
  })
}

export function cloudPut<T>(path: string, body: unknown): Promise<T> {
  return cloudRequest<T>(path, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
}
