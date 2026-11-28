import { afterEach, beforeEach, expect, test } from 'bun:test'
import { canGrantAppPermission, getTrustedRendererPath, isTrustedRendererURL, setTrustedRendererPath } from './permissions'

const expectedPath = '/Applications/Jaz.app/Contents/Resources/app.asar/out/renderer/index.html'
const url = `file://${expectedPath}`
const originalPath = getTrustedRendererPath()
const contents = (value = url, type = 'window', destroyed = false) => ({
  getURL: () => value,
  getType: () => type,
  isDestroyed: () => destroyed,
})

beforeEach(() => {
  setTrustedRendererPath(expectedPath)
})

afterEach(() => {
  setTrustedRendererPath(originalPath)
})

test('isTrustedRendererURL matches only the exact configured renderer file path', () => {
  expect(isTrustedRendererURL(url)).toBe(true)
  expect(isTrustedRendererURL(`${url}#/sessions/1`)).toBe(true)
  expect(isTrustedRendererURL('file:///tmp/evil/renderer/index.html')).toBe(false)
  expect(isTrustedRendererURL('file:///Applications/Jaz.app/Contents/Resources/app.asar/out/renderer/other.html')).toBe(false)
  expect(isTrustedRendererURL('https://untrusted.example/renderer/index.html')).toBe(false)
})

test('clipboard writes trust the containing Jaz window, including opaque embedded apps', () => {
  expect(canGrantAppPermission(contents(), 'clipboard-sanitized-write', { isMainFrame: true, requestingUrl: url })).toBe(true)
  expect(canGrantAppPermission(contents(), 'clipboard-sanitized-write', { isMainFrame: false, requestingUrl: 'about:srcdoc' })).toBe(true)
  expect(canGrantAppPermission(contents(), 'clipboard-sanitized-write', { isMainFrame: false })).toBe(true)
  for (const sender of [null, contents(url, 'window', true), contents(url, 'webview'), contents('https://untrusted.example/'), contents('file:///tmp/evil/renderer/index.html')]) {
    expect(canGrantAppPermission(sender, 'clipboard-sanitized-write', { isMainFrame: false, requestingUrl: url })).toBe(false)
  }
})

test('clipboard reads stay denied and media and fonts stay in the trusted main frame', () => {
  for (const isMainFrame of [true, false]) {
    for (const permission of ['clipboard-read', 'deprecated-sync-clipboard-read', 'clipboard-write', 'notifications']) {
      expect(canGrantAppPermission(contents(), permission, { isMainFrame, requestingUrl: url })).toBe(false)
    }
    for (const permission of ['media', 'local-fonts']) {
      expect(canGrantAppPermission(contents(), permission, { isMainFrame, requestingUrl: url })).toBe(isMainFrame)
      expect(canGrantAppPermission(contents(), permission, { isMainFrame, requestingUrl: 'https://untrusted.example/' })).toBe(false)
    }
  }
})
