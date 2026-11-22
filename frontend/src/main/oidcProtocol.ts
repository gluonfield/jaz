import type { BrowserWindow } from 'electron'
import { isTrustedRendererURL } from './permissions'

export const OIDC_BEGIN_CHANNEL = 'jaz:oidc-begin'
export const OIDC_CALLBACK_CHANNEL = 'jaz:oidc-callback'
export const OIDC_GATE_TTL_MS = 10 * 60_000

export function isOidcCallbackUrl(raw: string): boolean {
  try {
    const parsed = new URL(raw.trim())
    return (
      parsed.protocol === 'jaz:' &&
      parsed.hostname === 'auth' &&
      parsed.pathname === '/callback' &&
      parsed.hash.length > 1
    )
  } catch {
    return false
  }
}

export function extractOidcCallbackArg(argv: readonly string[]): string | null {
  for (const arg of argv) {
    if (typeof arg === 'string' && isOidcCallbackUrl(arg)) {
      return arg.trim()
    }
  }
  return null
}

type PendingGate = {
  ownerWindowId: number
  expiresAt: number
}

// Gates Electron jaz://auth/callback delivery on an explicit in-flight sign-in
// started by the trusted main window. Unsolicited or expired callbacks are dropped.
export function createOidcCallbackGate({
  ttlMs = OIDC_GATE_TTL_MS,
  now = Date.now,
}: {
  ttlMs?: number
  now?: () => number
} = {}) {
  let pending: PendingGate | null = null

  function begin(ownerWindowId: number): void {
    pending = { ownerWindowId, expiresAt: now() + ttlMs }
  }

  function clear(): void {
    pending = null
  }

  function consume(rawUrl: string, targetWindow: BrowserWindow | null): boolean {
    if (!isOidcCallbackUrl(rawUrl)) {
      return false
    }
    const active = pending
    pending = null
    if (!active || now() > active.expiresAt) {
      return false
    }
    if (!targetWindow || targetWindow.isDestroyed()) {
      return false
    }
    const contents = targetWindow.webContents
    if (
      contents.isDestroyed() ||
      contents.id !== active.ownerWindowId ||
      contents.getType() !== 'window' ||
      !isTrustedRendererURL(contents.mainFrame?.url)
    ) {
      return false
    }
    if (targetWindow.isMinimized?.()) targetWindow.restore?.()
    targetWindow.show()
    targetWindow.focus()
    contents.mainFrame.send(OIDC_CALLBACK_CHANNEL, rawUrl.trim())
    return true
  }

  return { begin, clear, consume }
}
