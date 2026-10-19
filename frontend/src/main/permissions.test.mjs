import { expect, test } from 'bun:test'
import { canGrantAppPermission } from './permissions'

const url = 'file:///Applications/Jaz.app/Contents/Resources/app.asar/out/renderer/index.html'
const contents = (value = url, type = 'window', destroyed = false) => ({
  getURL: () => value,
  getType: () => type,
  isDestroyed: () => destroyed,
})

test('clipboard writes trust the containing Jaz window, including opaque embedded apps', () => {
  expect(canGrantAppPermission(contents(), 'clipboard-sanitized-write', { isMainFrame: true, requestingUrl: url })).toBe(true)
  expect(canGrantAppPermission(contents(), 'clipboard-sanitized-write', { isMainFrame: false, requestingUrl: 'about:srcdoc' })).toBe(true)
  expect(canGrantAppPermission(contents(), 'clipboard-sanitized-write', { isMainFrame: false })).toBe(true)
  for (const sender of [null, contents(url, 'window', true), contents(url, 'webview'), contents('https://untrusted.example/')]) {
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
