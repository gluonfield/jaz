import { readAPIResponse } from '@/lib/api/response'
import { backendFetch, clientRuntime, openExternal } from '@/lib/clientRuntime'
import type { ResourceTicket } from '@shared/connectionSession'
import {
  CLIENT_PLATFORM,
  CLIENT_PLATFORM_HEADER,
  normalizeBaseUrl,
  secureBackendConnection,
} from './client'
import { cloudGet } from './cloudClient'
import type { CloudUser } from './serverTypes'

export const PENDING_OIDC_STORAGE_KEY = 'jaz.oidc.pending'
export const PENDING_OIDC_TTL_MS = 10 * 60_000

export type OidcLoginPurpose = 'primary' | 'attached'

export interface PendingOidcLogin {
  server: string
  state: string
  codeVerifier?: string
  startedAt: number
  purpose?: OidcLoginPurpose
}

export interface ParsedOidcCallback {
  code: string
  token: string
  state: string
  server: string
}

export type ValidatedOidcCallback =
  | { ok: true; server: string; code: string; token: string; codeVerifier?: string }
  | { ok: false; error: string }

export interface AuthMeResult {
  user: CloudUser
  ticket?: ResourceTicket
}

export interface AuthExchangeResult {
  token: string
  user: CloudUser
  ticket?: ResourceTicket
}

function base64UrlEncode(bytes: Uint8Array): string {
  let binary = ''
  for (const b of bytes) binary += String.fromCharCode(b)
  return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')
}

function generateRandomUrlSafe(byteLength = 32): string {
  const bytes = new Uint8Array(byteLength)
  crypto.getRandomValues(bytes)
  return base64UrlEncode(bytes)
}

export async function pkceS256Challenge(verifier: string): Promise<string> {
  const data = new TextEncoder().encode(verifier)
  const digest = await crypto.subtle.digest('SHA-256', data)
  return base64UrlEncode(new Uint8Array(digest))
}

export function beginPendingOidcLogin(
  server: string,
  now: () => number = Date.now,
  purpose?: OidcLoginPurpose,
): PendingOidcLogin {
  const origin = normalizeBaseUrl(server)
  if (!secureBackendConnection(origin)) {
    throw new Error('Remote connections require HTTPS')
  }
  const record: PendingOidcLogin = {
    server: origin,
    state: generateRandomUrlSafe(24),
    codeVerifier: generateRandomUrlSafe(32),
    startedAt: now(),
    ...(purpose ? { purpose } : {}),
  }
  try {
    sessionStorage.setItem(PENDING_OIDC_STORAGE_KEY, JSON.stringify(record))
  } catch {
    // sessionStorage may be unavailable in restricted contexts.
  }
  return record
}

function parsePendingRecord(raw: string | null, now: () => number): PendingOidcLogin | null {
  if (!raw) return null
  try {
    const parsed = JSON.parse(raw) as Partial<PendingOidcLogin> | null
    if (
      !parsed ||
      typeof parsed.server !== 'string' ||
      typeof parsed.state !== 'string' ||
      typeof parsed.startedAt !== 'number' ||
      !parsed.server.trim() ||
      !parsed.state.trim() ||
      !Number.isFinite(parsed.startedAt)
    ) {
      return null
    }
    const elapsed = now() - parsed.startedAt
    if (elapsed < -60_000 || elapsed > PENDING_OIDC_TTL_MS) {
      return null
    }
    const server = normalizeBaseUrl(parsed.server)
    if (!secureBackendConnection(server)) {
      return null
    }
    const codeVerifier = typeof parsed.codeVerifier === 'string' && parsed.codeVerifier.trim()
      ? parsed.codeVerifier.trim()
      : undefined
    const purpose: OidcLoginPurpose | undefined =
      parsed.purpose === 'attached' || parsed.purpose === 'primary' ? parsed.purpose : undefined
    return {
      server,
      state: parsed.state.trim(),
      ...(codeVerifier ? { codeVerifier } : {}),
      startedAt: parsed.startedAt,
      ...(purpose ? { purpose } : {}),
    }
  } catch {
    return null
  }
}

export function peekPendingOidcLogin(now: () => number = Date.now): PendingOidcLogin | null {
  try {
    const raw = sessionStorage.getItem(PENDING_OIDC_STORAGE_KEY)
    const record = parsePendingRecord(raw, now)
    if (!record && raw !== null) {
      sessionStorage.removeItem(PENDING_OIDC_STORAGE_KEY)
    }
    return record
  } catch {
    return null
  }
}

export function consumePendingOidcLogin(now: () => number = Date.now): PendingOidcLogin | null {
  try {
    const raw = sessionStorage.getItem(PENDING_OIDC_STORAGE_KEY)
    sessionStorage.removeItem(PENDING_OIDC_STORAGE_KEY)
    return parsePendingRecord(raw, now)
  } catch {
    return null
  }
}

export function clearPendingOidcLogin(): void {
  try {
    sessionStorage.removeItem(PENDING_OIDC_STORAGE_KEY)
  } catch {
    // ignore
  }
}

export function buildOidcLoginUrl(
  baseUrl: string,
  oidcLoginUrl = '/v1/auth/login',
  options: { clientRedirect?: string; state?: string; codeChallenge?: string } = {},
): string {
  const origin = normalizeBaseUrl(baseUrl)
  if (!secureBackendConnection(origin)) {
    throw new Error('Remote connections require HTTPS')
  }
  const rawPath = oidcLoginUrl.trim() || '/v1/auth/login'
  if (rawPath.startsWith('//') || rawPath.includes('\\')) {
    throw new Error('OIDC login URL must stay on the server origin')
  }
  let url: URL
  try {
    url = new URL(rawPath, `${origin}/`)
  } catch {
    throw new Error('Invalid OIDC login URL')
  }
  if (url.origin !== origin || !secureBackendConnection(url.origin)) {
    throw new Error('OIDC login URL must match the server origin')
  }
  if (options.clientRedirect?.trim()) {
    url.searchParams.set('client_redirect', options.clientRedirect.trim())
  }
  if (options.state?.trim()) {
    url.searchParams.set('state', options.state.trim())
  }
  if (options.codeChallenge?.trim()) {
    url.searchParams.set('code_challenge', options.codeChallenge.trim())
  }
  return url.toString()
}

// Parses OIDC callback parameters strictly from the URL fragment (#code=, #token=, #state=, #server=).
// Supports https://..., http://..., jaz://auth/callback#..., and raw #... fragments.
// Query-string credentials (?token= / ?key= / ?code=) are intentionally ignored.
export function parseOidcCallbackUrl(input: string): ParsedOidcCallback {
  const trimmed = input.trim()
  if (!trimmed) {
    return { code: '', token: '', state: '', server: '' }
  }
  let fragment = ''
  if (/^(?:https?|jaz):\/\//i.test(trimmed)) {
    try {
      const parsed = new URL(trimmed)
      fragment = parsed.hash.replace(/^#\??/, '')
    } catch {
      return { code: '', token: '', state: '', server: '' }
    }
  } else if (trimmed.startsWith('#')) {
    fragment = trimmed.replace(/^#\??/, '')
  } else if (/^(?:code|token|state|server|url)=/.test(trimmed)) {
    fragment = trimmed
  } else {
    return { code: '', token: '', state: '', server: '' }
  }
  const params = new URLSearchParams(fragment)
  return {
    code: params.get('code')?.trim() ?? '',
    token: params.get('token')?.trim() ?? '',
    state: params.get('state')?.trim() ?? '',
    server: (params.get('server') || params.get('url'))?.trim() ?? '',
  }
}

export function validateOidcCallback(
  input: string,
  pending: PendingOidcLogin | null,
  now: () => number = Date.now,
): ValidatedOidcCallback {
  if (!pending) {
    return { ok: false, error: 'No active SSO sign-in request found. Start sign-in from Jaz.' }
  }
  const elapsed = now() - pending.startedAt
  if (elapsed < -60_000 || elapsed > PENDING_OIDC_TTL_MS) {
    return { ok: false, error: 'SSO sign-in request expired. Please start sign-in again.' }
  }
  const expectedServer = normalizeBaseUrl(pending.server)
  if (!secureBackendConnection(expectedServer)) {
    return { ok: false, error: 'Remote connections require HTTPS' }
  }
  const parsed = parseOidcCallbackUrl(input)
  if (!parsed.code && !parsed.token) {
    return { ok: false, error: 'Callback did not include an authorization code or session token' }
  }
  if (parsed.server) {
    const callbackServer = normalizeBaseUrl(parsed.server)
    if (callbackServer !== expectedServer) {
      return { ok: false, error: 'OIDC callback server does not match the server you signed into' }
    }
  }
  if (!parsed.state || parsed.state !== pending.state) {
    return { ok: false, error: 'OIDC state mismatch. Please start sign-in again.' }
  }
  return {
    ok: true,
    server: expectedServer,
    code: parsed.code,
    token: parsed.token,
    ...(pending.codeVerifier ? { codeVerifier: pending.codeVerifier } : {}),
  }
}

type RawResourceTicket =
  | string
  | {
      token?: string
      expires_at?: string
    }

type RawAuthMe = {
  id?: string
  user_id?: string
  org_id?: string
  org_name?: string
  email?: string
  display_name?: string
  name?: string
  role?: string
  user?: RawAuthMe
  resource_ticket?: RawResourceTicket
}

function jwtExpiresAtISO(token: string): string {
  try {
    const parts = token.split('.')
    if (parts.length === 3 && parts[1]) {
      const padded = parts[1].replace(/-/g, '+').replace(/_/g, '/') + '==='.slice((parts[1].length + 3) % 4)
      const parsed = JSON.parse(atob(padded)) as { exp?: unknown }
      if (typeof parsed.exp === 'number' && Number.isFinite(parsed.exp) && parsed.exp > 0) {
        return new Date(parsed.exp * 1000).toISOString()
      }
    }
  } catch {
    // Fallback below if token is opaque.
  }
  return new Date(Date.now() + 55 * 60_000).toISOString()
}

export function extractResourceTicket(raw: { resource_ticket?: RawResourceTicket } | null | undefined): ResourceTicket | undefined {
  const ticket = raw?.resource_ticket
  if (typeof ticket === 'string' && ticket.trim()) {
    const trimmed = ticket.trim()
    return { token: trimmed, expires_at: jwtExpiresAtISO(trimmed) }
  }
  if (ticket && typeof ticket === 'object' && typeof ticket.token === 'string' && ticket.token.trim()) {
    const token = ticket.token.trim()
    const expiresAt = typeof ticket.expires_at === 'string' && ticket.expires_at.trim()
      ? ticket.expires_at.trim()
      : jwtExpiresAtISO(token)
    return { token, expires_at: expiresAt }
  }
  return undefined
}

export function normalizeCloudUser(raw: RawAuthMe): CloudUser {
  const source = raw.user ?? raw
  const id = (source.id ?? source.user_id ?? '').trim()
  const orgId = (source.org_id ?? '').trim()
  const email = (source.email ?? '').trim()
  const displayName = (source.display_name ?? source.name ?? '').trim() || email
  const role = (source.role ?? 'member').trim() || 'member'
  if (!id && !email && !orgId) {
    throw new Error('Cloud server returned an invalid user profile')
  }
  return {
    id,
    org_id: orgId,
    ...(source.org_name?.trim() ? { org_name: source.org_name.trim() } : {}),
    email,
    display_name: displayName,
    role,
  }
}

export async function launchOidcBrowserFlow(
  origin: string,
  loginPath: string,
  pending: PendingOidcLogin,
): Promise<void> {
  const codeChallenge = pending.codeVerifier ? await pkceS256Challenge(pending.codeVerifier) : undefined
  if (clientRuntime.kind === 'web') {
    const sameOrigin = window.location.origin === origin
    const clientRedirect = sameOrigin ? undefined : `${window.location.origin}/`
    const target = buildOidcLoginUrl(origin, loginPath, {
      clientRedirect,
      state: pending.state,
      ...(clientRedirect && codeChallenge ? { codeChallenge } : {}),
    })
    if (typeof window.location.assign === 'function') {
      window.location.assign(target)
    } else {
      openExternal(target)
    }
    return
  }
  const started = await clientRuntime.beginOidcSignIn?.()
  const clientRedirect =
    started && typeof started === 'object' && typeof started.redirectUri === 'string' && started.redirectUri.trim()
      ? started.redirectUri.trim()
      : 'jaz://auth/callback'
  const target = buildOidcLoginUrl(origin, loginPath, {
    clientRedirect,
    state: pending.state,
    ...(codeChallenge ? { codeChallenge } : {}),
  })
  openExternal(target)
}

export async function fetchAuthMe(
  baseUrl?: string,
  options: { token?: string; signal?: AbortSignal; credentials?: RequestCredentials } = {},
): Promise<AuthMeResult> {
  if (!baseUrl) {
    const raw = await cloudGet<RawAuthMe>('/v1/auth/me', { redirect: 'error', signal: options.signal })
    return { user: normalizeCloudUser(raw), ticket: extractResourceTicket(raw) }
  }
  const origin = normalizeBaseUrl(baseUrl)
  if (!secureBackendConnection(origin)) {
    throw new Error('Remote connections require HTTPS')
  }
  const headers = new Headers({ [CLIENT_PLATFORM_HEADER]: CLIENT_PLATFORM })
  if (options.token) {
    headers.set('Authorization', `Bearer ${options.token}`)
  }
  const response = await backendFetch(`${origin}/v1/auth/me`, {
    headers,
    credentials: options.credentials,
    redirect: 'error',
    signal: options.signal,
  })
  const raw = await readAPIResponse<RawAuthMe>(response)
  return { user: normalizeCloudUser(raw), ticket: extractResourceTicket(raw) }
}

type RawExchangeResponse = {
  token?: string
  user?: RawAuthMe
  resource_ticket?: RawResourceTicket
}

export async function exchangeOidcCode(
  baseUrl: string,
  code: string,
  signal?: AbortSignal,
  codeVerifier?: string,
): Promise<AuthExchangeResult> {
  const origin = normalizeBaseUrl(baseUrl)
  if (!secureBackendConnection(origin)) {
    throw new Error('Remote connections require HTTPS')
  }
  const trimmedCode = code.trim()
  if (!trimmedCode) {
    throw new Error('Authorization code is required')
  }
  const payload: { code: string; code_verifier?: string } = { code: trimmedCode }
  if (codeVerifier?.trim()) {
    payload.code_verifier = codeVerifier.trim()
  }
  const response = await backendFetch(`${origin}/v1/auth/exchange`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      [CLIENT_PLATFORM_HEADER]: CLIENT_PLATFORM,
    },
    body: JSON.stringify(payload),
    redirect: 'error',
    signal,
  })
  const raw = await readAPIResponse<RawExchangeResponse>(response)
  const token = raw.token?.trim() ?? ''
  if (!token) {
    throw new Error('Cloud server did not return a session token')
  }
  const user = raw.user
    ? normalizeCloudUser(raw.user)
    : (await fetchAuthMe(origin, { token, signal })).user
  return {
    token,
    user,
    ticket: extractResourceTicket(raw),
  }
}

export async function logoutCloudSession(baseUrl: string, token?: string): Promise<void> {
  const origin = normalizeBaseUrl(baseUrl)
  if (!secureBackendConnection(origin)) return
  const headers = new Headers({ [CLIENT_PLATFORM_HEADER]: CLIENT_PLATFORM })
  if (token?.trim()) {
    headers.set('Authorization', `Bearer ${token.trim()}`)
  }
  try {
    await backendFetch(`${origin}/v1/auth/logout`, {
      method: 'POST',
      headers,
      credentials: !token?.trim() && typeof window !== 'undefined' ? 'include' : undefined,
      redirect: 'error',
      signal: AbortSignal.timeout(3000),
    })
  } catch {
    // Best-effort revocation; local session still clears immediately.
  }
}
