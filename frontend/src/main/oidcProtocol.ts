import { createServer, type IncomingMessage, type Server, type ServerResponse } from 'node:http'
import type { AddressInfo } from 'node:net'
import type { BrowserWindow } from 'electron'
import { isTrustedRendererURL } from './permissions'

export const OIDC_BEGIN_CHANNEL = 'jaz:oidc-begin'
export const OIDC_CALLBACK_CHANNEL = 'jaz:oidc-callback'
export const OIDC_GATE_TTL_MS = 10 * 60_000
const MAX_CALLBACK_BODY_BYTES = 8 * 1024

const LOOPBACK_CALLBACK_HTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8" />
<title>Jaz Sign-In</title>
<style>
  :root { color-scheme: light dark; }
  body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; display: grid; place-items: center; min-height: 100vh; margin: 0; background: Canvas; color: CanvasText; }
  main { text-align: center; padding: 24px; max-width: 360px; }
  p { opacity: 0.72; font-size: 14px; line-height: 1.5; }
</style>
</head>
<body>
<main>
  <h2 id="title">Completing sign-in…</h2>
  <p id="detail">Returning to Jaz.</p>
</main>
<script>
(async () => {
  const title = document.getElementById('title');
  const detail = document.getElementById('detail');
  const hash = window.location.hash || '';
  try { history.replaceState(null, '', '/auth/callback'); } catch {}
  if (!hash || hash.length <= 1) {
    title.textContent = 'Sign-in could not be completed';
    detail.textContent = 'No authorization payload was returned. Try signing in again from Jaz.';
    return;
  }
  try {
    const res = await fetch('/auth/callback', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ hash }),
    });
    if (!res.ok) throw new Error('rejected');
    title.textContent = 'Signed in to Jaz';
    detail.textContent = 'You can close this tab and return to Jaz.';
    setTimeout(() => { try { window.close(); } catch {} }, 300);
  } catch {
    title.textContent = 'Sign-in expired';
    detail.textContent = 'Return to Jaz and start sign-in again.';
  }
})();
</script>
</body>
</html>`

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

function readLimitedBody(req: IncomingMessage, maxBytes: number): Promise<string | null> {
  return new Promise((resolve) => {
    const chunks: Buffer[] = []
    let total = 0
    let done = false
    const finish = (value: string | null) => {
      if (done) return
      done = true
      resolve(value)
    }
    req.on('data', (chunk: Buffer) => {
      total += chunk.length
      if (total > maxBytes) {
        req.destroy()
        finish(null)
        return
      }
      chunks.push(chunk)
    })
    req.on('end', () => finish(Buffer.concat(chunks).toString('utf8')))
    req.on('error', () => finish(null))
  })
}

type PendingGate = {
  ownerWindowId: number
  expiresAt: number
}

// Gates Electron OIDC callback delivery on an explicit in-flight sign-in
// started by the trusted main window. Supports both RFC 8252 ephemeral
// loopback callbacks (http://127.0.0.1:<port>/auth/callback) and jaz://auth/callback.
export function createOidcCallbackGate({
  ttlMs = OIDC_GATE_TTL_MS,
  now = Date.now,
}: {
  ttlMs?: number
  now?: () => number
} = {}) {
  let pending: PendingGate | null = null
  let loopbackServer: Server | null = null
  let loopbackTimer: ReturnType<typeof setTimeout> | null = null

  function closeLoopback(): void {
    if (loopbackTimer) {
      clearTimeout(loopbackTimer)
      loopbackTimer = null
    }
    const server = loopbackServer
    loopbackServer = null
    if (server) {
      try {
        server.close()
      } catch {
        // already closed
      }
    }
  }

  function begin(ownerWindowId: number): void {
    closeLoopback()
    pending = { ownerWindowId, expiresAt: now() + ttlMs }
  }

  function clear(): void {
    pending = null
    closeLoopback()
  }

  function consume(rawUrl: string, targetWindow: BrowserWindow | null): boolean {
    if (!isOidcCallbackUrl(rawUrl)) {
      return false
    }
    const active = pending
    pending = null
    closeLoopback()
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

  async function beginLoopback(
    ownerWindowId: number,
    getTargetWindow: () => BrowserWindow | null,
  ): Promise<string> {
    begin(ownerWindowId)
    let expectedOrigin = ''
    const server = createServer((req: IncomingMessage, res: ServerResponse) => {
      const path = (req.url ?? '').split('?')[0]
      res.setHeader('Cache-Control', 'no-store')
      res.setHeader('Referrer-Policy', 'no-referrer')
      res.setHeader('X-Frame-Options', 'DENY')
      if (req.method === 'GET' && path === '/auth/callback') {
        res.writeHead(200, {
          'Content-Type': 'text/html; charset=utf-8',
          'Content-Security-Policy': "default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; connect-src 'self'",
        })
        res.end(LOOPBACK_CALLBACK_HTML)
        return
      }
      if (req.method === 'POST' && path === '/auth/callback') {
        if (req.headers.origin !== expectedOrigin) {
          res.writeHead(403, { 'Content-Type': 'application/json' })
          res.end('{"error":"forbidden origin"}')
          return
        }
        void readLimitedBody(req, MAX_CALLBACK_BODY_BYTES).then((raw) => {
          if (!raw) {
            res.writeHead(400, { 'Content-Type': 'application/json' })
            res.end('{"error":"invalid body"}')
            return
          }
          let hash = ''
          try {
            const parsed = JSON.parse(raw) as { hash?: unknown }
            hash = typeof parsed.hash === 'string' ? parsed.hash.trim() : ''
          } catch {
            hash = ''
          }
          if (!hash.startsWith('#') || hash.length <= 1) {
            res.writeHead(400, { 'Content-Type': 'application/json' })
            res.end('{"error":"missing fragment"}')
            return
          }
          const delivered = consume(`jaz://auth/callback${hash}`, getTargetWindow())
          res.writeHead(delivered ? 200 : 400, { 'Content-Type': 'application/json' })
          res.end(delivered ? '{"ok":true}' : '{"error":"callback rejected"}')
        })
        return
      }
      res.writeHead(404, { 'Content-Type': 'text/plain' })
      res.end('not found')
    })

    await new Promise<void>((resolve, reject) => {
      server.once('error', reject)
      server.listen(0, '127.0.0.1', () => {
        server.removeListener('error', reject)
        resolve()
      })
    })
    const addr = server.address() as AddressInfo | null
    if (!addr || typeof addr.port !== 'number') {
      server.close()
      throw new Error('Could not bind local OIDC callback listener')
    }
    expectedOrigin = `http://127.0.0.1:${addr.port}`
    loopbackServer = server
    loopbackTimer = setTimeout(() => closeLoopback(), ttlMs)
    loopbackTimer.unref?.()
    return `${expectedOrigin}/auth/callback`
  }

  return { begin, beginLoopback, clear, consume }
}
