import { refreshCloudSession, type AuthRefreshResult } from '@/lib/api/cloudAuth'
import { ApiError } from '@/lib/api/response'
import { rememberApiAuthToken, rotateApiSessionTokens, type SessionRefreshOutcome } from '@/lib/api/client'
import type { CloudUser } from '@/lib/api/serverTypes'
import { forgetBrowserCredential, saveBrowserCredential } from '@/lib/browserCredentials'
import { clientRuntime } from '@/lib/clientRuntime'

export type StartupCloudRefreshResult =
  | { ok: true; refreshed: AuthRefreshResult }
  | { ok: false; unauthorized: boolean; recoverable: boolean; error: string }

export function persistRotatedCredential(
  origin: string,
  token: string,
  refreshToken: string | undefined,
  remembered: boolean,
  isCurrent: () => boolean,
): void {
  if (!token) return
  if (clientRuntime.kind === 'web') {
    void saveBrowserCredential(origin, token, isCurrent, refreshToken).catch(() => undefined)
    return
  }
  if (!remembered) return
  try {
    rememberApiAuthToken(origin, isCurrent)
  } catch {
    // Preserve the live refreshed session if secure storage is temporarily locked.
  }
}

export async function tryStartupCloudRefresh(
  origin: string,
  options: { refreshToken?: string; signal: AbortSignal },
): Promise<StartupCloudRefreshResult> {
  const mode = options.refreshToken ? 'bearer' : 'cookie'
  try {
    const refreshed = await refreshCloudSession(
      origin,
      mode === 'cookie'
        ? { credentials: 'include', signal: options.signal }
        : { refreshToken: options.refreshToken, signal: options.signal },
    )
    return { ok: true, refreshed }
  } catch (error) {
    if (error instanceof ApiError && error.status === 401) {
      if (clientRuntime.kind === 'web' && mode === 'bearer') {
        void forgetBrowserCredential(origin).catch(() => undefined)
      }
      return { ok: false, unauthorized: true, recoverable: false, error: 'Session expired. Sign in again.' }
    }
    const recoverable = !(error instanceof ApiError) || error.status >= 500
    const message = error instanceof Error ? error.message : 'Cloud session refresh failed'
    return { ok: false, unauthorized: false, recoverable, error: message }
  }
}

export async function runActiveCloudRefresh(params: {
  origin: string
  mode: 'cookie' | 'bearer'
  refreshToken: string
  signal: AbortSignal
  isCurrent: () => boolean
  ownsConnection: boolean
  wasRemembered: boolean
  onUserUpdated: (user: CloudUser) => void
  onBearerRotated: (token: string) => void
}): Promise<SessionRefreshOutcome> {
  const { origin, mode, refreshToken, signal, isCurrent, ownsConnection, wasRemembered, onUserUpdated, onBearerRotated } = params
  try {
    const refreshed = await refreshCloudSession(
      origin,
      mode === 'cookie' ? { credentials: 'include', signal } : { refreshToken, signal },
    )
    if (!isCurrent()) {
      return 'error'
    }
    if (mode === 'cookie') {
      rotateApiSessionTokens(origin, { ticket: refreshed.ticket })
      onUserUpdated(refreshed.user)
      return 'refreshed'
    }
    const nextRefresh = refreshed.refreshToken ?? refreshToken
    rotateApiSessionTokens(origin, {
      token: refreshed.token,
      refreshToken: nextRefresh,
      ticket: refreshed.ticket,
    })
    onBearerRotated(refreshed.token)
    onUserUpdated(refreshed.user)
    if (ownsConnection) {
      persistRotatedCredential(origin, refreshed.token, nextRefresh, wasRemembered, isCurrent)
    }
    return 'refreshed'
  } catch (error) {
    if (error instanceof ApiError && error.status === 401) {
      if (clientRuntime.kind === 'web' && mode === 'bearer') {
        void forgetBrowserCredential(origin).catch(() => undefined)
      }
      return 'unauthorized'
    }
    return 'error'
  }
}
