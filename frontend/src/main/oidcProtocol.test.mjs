import { afterEach, beforeEach, expect, test } from 'bun:test'
import {
  OIDC_CALLBACK_CHANNEL,
  createOidcCallbackGate,
  extractOidcCallbackArg,
  isOidcCallbackUrl,
} from './oidcProtocol'
import { getTrustedRendererPath, setTrustedRendererPath } from './permissions'

const expectedPath = '/Applications/Jaz.app/Contents/Resources/app.asar/out/renderer/index.html'
const trustedUrl = `file://${expectedPath}`
const originalPath = getTrustedRendererPath()

beforeEach(() => {
  setTrustedRendererPath(expectedPath)
})

afterEach(() => {
  setTrustedRendererPath(originalPath)
})

function mockWindow({ id = 1, url = trustedUrl, type = 'window', destroyed = false, minimized = false } = {}) {
  const sent = []
  const actions = []
  return {
    sent,
    actions,
    isDestroyed: () => destroyed,
    isMinimized: () => minimized,
    restore: () => actions.push('restore'),
    show: () => actions.push('show'),
    focus: () => actions.push('focus'),
    webContents: {
      id,
      isDestroyed: () => destroyed,
      getType: () => type,
      mainFrame: {
        url,
        send: (channel, payload) => sent.push([channel, payload]),
      },
    },
  }
}

test('isOidcCallbackUrl and extractOidcCallbackArg accept only jaz://auth/callback with fragment', () => {
  const valid = 'jaz://auth/callback#code=one-time-code&state=state-123'
  expect(isOidcCallbackUrl(valid)).toBe(true)
  expect(isOidcCallbackUrl('jaz://auth/callback')).toBe(false)
  expect(isOidcCallbackUrl('jaz://auth/callback?code=query-leak')).toBe(false)
  expect(isOidcCallbackUrl('jaz://other/callback#code=1')).toBe(false)
  expect(isOidcCallbackUrl('https://cloud.example/callback#code=1')).toBe(false)
  expect(extractOidcCallbackArg(['Jaz.exe', '--flag', valid])).toBe(valid)
  expect(extractOidcCallbackArg(['Jaz.exe', 'jaz://auth/callback?code=nope'])).toBeNull()
})

test('createOidcCallbackGate enforces single-use delivery, TTL, owner window, and trusted renderer URL', () => {
  let clock = 1_000
  const gate = createOidcCallbackGate({ ttlMs: 60_000, now: () => clock })
  const callback = 'jaz://auth/callback#code=abc&state=xyz'

  const win = mockWindow({ id: 42, minimized: true })
  // Unsolicited callback before begin() is dropped.
  expect(gate.consume(callback, win)).toBe(false)
  expect(win.sent).toEqual([])

  // Expired callback is dropped.
  gate.begin(42)
  clock += 60_001
  expect(gate.consume(callback, win)).toBe(false)
  expect(win.sent).toEqual([])

  // Wrong window ID or untrusted renderer URL is dropped.
  gate.begin(42)
  const wrongWindow = mockWindow({ id: 99 })
  expect(gate.consume(callback, wrongWindow)).toBe(false)
  expect(wrongWindow.sent).toEqual([])

  gate.begin(42)
  const evilWindow = mockWindow({ id: 42, url: 'file:///tmp/evil/renderer/index.html' })
  expect(gate.consume(callback, evilWindow)).toBe(false)
  expect(evilWindow.sent).toEqual([])

  // Valid callback delivers once and replay is rejected.
  gate.begin(42)
  expect(gate.consume(callback, win)).toBe(true)
  expect(win.actions).toEqual(['restore', 'show', 'focus'])
  expect(win.sent).toEqual([[OIDC_CALLBACK_CHANNEL, callback]])
  expect(gate.consume(callback, win)).toBe(false)
  expect(win.sent).toHaveLength(1)
})

test('createOidcCallbackGate loopback server accepts same-origin fragment POST, focuses window, and closes once consumed', async () => {
  const gate = createOidcCallbackGate({ ttlMs: 60_000 })
  const win = mockWindow({ id: 7, minimized: true })
  const redirectUri = await gate.beginLoopback(7, () => win)
  const origin = new URL(redirectUri).origin
  expect(redirectUri).toMatch(/^http:\/\/127\.0\.0\.1:\d+\/auth\/callback$/)

  const page = await fetch(redirectUri)
  expect(page.status).toBe(200)
  expect(await page.text()).toContain('window.location.hash')

  const badOrigin = await fetch(redirectUri, {
    method: 'POST',
    headers: { Origin: 'https://evil.example', 'Content-Type': 'application/json' },
    body: JSON.stringify({ hash: '#code=evil&state=x' }),
  })
  expect(badOrigin.status).toBe(403)
  expect(win.sent).toHaveLength(0)

  const ok = await fetch(redirectUri, {
    method: 'POST',
    headers: { Origin: origin, 'Content-Type': 'application/json' },
    body: JSON.stringify({ hash: '#code=loopback-code&state=st-1' }),
  })
  expect(ok.status).toBe(200)
  expect(win.actions).toEqual(['restore', 'show', 'focus'])
  expect(win.sent).toEqual([[OIDC_CALLBACK_CHANNEL, 'jaz://auth/callback#code=loopback-code&state=st-1']])
})
