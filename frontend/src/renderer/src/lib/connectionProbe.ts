import { fetchAuthMe } from '@/lib/api/cloudAuth'
import type {
  CloudUser,
  ServerAuthMethod,
  ServerCapabilities,
  ServerMode,
} from '@/lib/api/serverTypes'
import { CLIENT_PLATFORM, CLIENT_PLATFORM_HEADER, normalizeBaseUrl, secureBackendConnection } from '@/lib/api/client'
import { ApiError } from '@/lib/api/response'
import { backendFetch, clientRuntime } from '@/lib/clientRuntime'
import { verifiedAccess, type AuthKind, type ResourceTicket } from '@shared/connectionSession'

export type ProbeFailure = { ok: false; error: string; recoverable: boolean; code?: string }
export type Access = { ok: true; kind: AuthKind; ticket?: ResourceTicket; user?: CloudUser } | ProbeFailure
export type Health = { ok: true; authRequired: boolean; capabilities?: ServerCapabilities } | ProbeFailure
export type ServerDiscovery =
  | {
      ok: true
      mode: ServerMode
      auth: ServerAuthMethod
      authRequired: boolean
      oidcLoginUrl: string
      oidcIssuer: string
      publicUrl: string
      capabilities: ServerCapabilities
    }
  | ProbeFailure

export async function readJSON<T>(response: Response): Promise<T | null> {
  try {
    return (await response.json()) as T
  } catch {
    return null
  }
}

function loginResponse(url: string, response: Response): boolean {
  if (clientRuntime.kind !== 'web') {
    return false
  }
  const contentType = response.headers.get('content-type') || ''
  return contentType.includes('text/html') || (response.redirected && new URL(response.url).origin !== url) ||
    ((response.status === 401 || response.status === 403) && !contentType.includes('json'))
}

function probeSignal(signal?: AbortSignal): AbortSignal {
  return signal ? AbortSignal.any([signal, AbortSignal.timeout(3000)]) : AbortSignal.timeout(3000)
}

export async function readHealth(url: string, signal?: AbortSignal): Promise<Health> {
  const origin = normalizeBaseUrl(url)
  if (!secureBackendConnection(origin)) {
    return { ok: false, error: 'Remote connections require HTTPS', recoverable: false }
  }
  try {
    const response = await backendFetch(origin + '/health', { signal: probeSignal(signal) })
    if (loginResponse(origin, response)) {
      return { ok: false, error: `Sign in through Teleport again to reach the backend at ${origin}`, recoverable: false }
    }
    const body = await readJSON<{ ok?: boolean; auth_required?: boolean; capabilities?: ServerCapabilities }>(response)
    if (response.ok && body?.ok === true && typeof body.auth_required === 'boolean') {
      return { ok: true, authRequired: body.auth_required, capabilities: body.capabilities ?? {} }
    }
  } catch {
    // A health failure never unlocks a credential or issues a replacement.
  }
  return { ok: false, error: `No backend responded at ${origin}`, recoverable: true }
}

type RawDiscoveryPayload = {
  ok?: boolean
  mode?: string
  auth?: string
  oidc_login_url?: string
  oidc_issuer?: string
  public_url?: string
  capabilities?: ServerCapabilities
}

function parseDiscoveryPayload(body: RawDiscoveryPayload | null): Extract<ServerDiscovery, { ok: true }> | null {
  if (!body || body.ok !== true) return null
  const mode: ServerMode | null = body.mode === 'cloud' || body.mode === 'local' ? body.mode : null
  const auth: ServerAuthMethod | null =
    body.auth === 'oidc' || body.auth === 'bearer' || body.auth === 'none' ? body.auth : null
  if (!mode || !auth) return null
  return {
    ok: true,
    mode,
    auth,
    authRequired: auth !== 'none',
    oidcLoginUrl: typeof body.oidc_login_url === 'string' && body.oidc_login_url.trim()
      ? body.oidc_login_url.trim()
      : auth === 'oidc'
        ? '/v1/auth/login'
        : '',
    oidcIssuer: typeof body.oidc_issuer === 'string' ? body.oidc_issuer.trim() : '',
    publicUrl: typeof body.public_url === 'string' ? body.public_url.trim() : '',
    capabilities: body.capabilities ?? {},
  }
}

export async function fetchServerDiscovery(baseUrl: string, signal?: AbortSignal): Promise<ServerDiscovery> {
  const origin = normalizeBaseUrl(baseUrl)
  if (!secureBackendConnection(origin)) {
    return { ok: false, error: 'Remote connections require HTTPS', recoverable: false }
  }
  try {
    const response = await backendFetch(`${origin}/.well-known/jaz-server`, { signal: probeSignal(signal) })
    const contentType = response.headers.get('content-type') || ''
    if (response.ok && !contentType.includes('text/html')) {
      const parsed = parseDiscoveryPayload(await readJSON<RawDiscoveryPayload>(response))
      if (parsed) return parsed
    } else if (response.status >= 500 && response.status !== 501) {
      return { ok: false, error: `No backend responded at ${origin}`, recoverable: true }
    }
  } catch {
    // Older local backends may not serve /.well-known/jaz-server; fall back to /health.
  }

  const health = await readHealth(origin, signal)
  if (!health.ok) return health
  return {
    ok: true,
    mode: 'local',
    auth: health.authRequired ? 'bearer' : 'none',
    authRequired: health.authRequired,
    oidcLoginUrl: '',
    oidcIssuer: '',
    publicUrl: '',
    capabilities: health.capabilities ?? {},
  }
}

export async function readAccess(url: string, token: string, signal?: AbortSignal): Promise<Access> {
  if (!token || !secureBackendConnection(url)) {
    return { ok: false, error: 'A secure credential is required', recoverable: false }
  }
  try {
    const response = await backendFetch(url + '/v1/auth/check', {
      headers: { Authorization: `Bearer ${token}`, [CLIENT_PLATFORM_HEADER]: CLIENT_PLATFORM },
      redirect: 'error',
      signal: probeSignal(signal),
    })
    const body = await readJSON<{ ok?: boolean; auth_kind?: string; error?: string; code?: string }>(response)
    if (!response.ok) {
      return {
        ok: false,
        error: response.status === 401 ? 'Backend key was rejected' : body?.error || 'Backend authentication failed',
        recoverable: response.status >= 500,
        code: body?.code,
      }
    }
    try {
      return { ok: true, ...verifiedAccess(body) }
    } catch (error) {
      return { ok: false, error: (error as Error).message, recoverable: false }
    }
  } catch {
    return { ok: false, error: 'Backend auth check failed', recoverable: true }
  }
}

export async function readCloudAccess(url: string, token: string, signal?: AbortSignal): Promise<Access> {
  const origin = normalizeBaseUrl(url)
  if (!secureBackendConnection(origin)) {
    return { ok: false, error: 'Remote connections require HTTPS', recoverable: false }
  }
  if (!token && clientRuntime.kind !== 'web') {
    return { ok: false, error: 'Sign in to connect to this server', recoverable: false, code: 'oidc_required' }
  }
  try {
    const { user, ticket } = await fetchAuthMe(origin, {
      token: token || undefined,
      credentials: !token && clientRuntime.kind === 'web' ? 'include' : undefined,
      signal: probeSignal(signal),
    })
    return { ok: true, kind: 'oidc', user, ticket }
  } catch (error) {
    if (error instanceof ApiError) {
      if (error.status === 401 || error.status === 403) {
        return {
          ok: false,
          error: token ? 'Session expired. Sign in again.' : 'Sign in to connect to this server',
          recoverable: false,
          code: 'oidc_required',
        }
      }
      return { ok: false, error: error.message || 'Cloud authentication failed', recoverable: error.status >= 500 }
    }
    return { ok: false, error: 'Cloud auth check failed', recoverable: true }
  }
}
