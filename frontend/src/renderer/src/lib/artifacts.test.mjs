import { expect, test } from 'bun:test'
import {
  artifactErrorForDisplay,
  buildArtifactCSP,
  buildArtifactDocument,
  resolvePluginAssetRelpath,
  sanitizeAssetOrigin,
} from './artifacts'

test('artifact iframe layout warnings stay internal while real errors remain visible', () => {
  expect(artifactErrorForDisplay('ResizeObserver loop completed with undelivered notifications.')).toBeUndefined()
  expect(artifactErrorForDisplay('ReferenceError: missingValue is not defined')).toBe('ReferenceError: missingValue is not defined')
})

test('a canvas card starts with its saved data, which cannot close the script that carries it', () => {
  const html = buildArtifactDocument({ title: 'card', code: '<p>card</p>', loadingMessages: [] }, '', { canvas: true, data: { note: '</script><b>\u2028' } })
  expect(html).toContain('window.data = {"note":"\\u003c/script>\\u003cb>\\u2028"}')
  expect(html.match(/<\/script>/g)).toHaveLength(1)
  expect(buildArtifactDocument({ title: 'card', code: '<p>card</p>', loadingMessages: [] }, '', { canvas: true })).toContain('window.data = null')
  expect(buildArtifactDocument({ title: 'card', code: '<p>card</p>', loadingMessages: [] }, '')).not.toContain('saveData')
})

test("a plugin's view starts with its data and is not treated as a canvas card", () => {
  const html = buildArtifactDocument({ title: 'status', code: '<p>status</p>', loadingMessages: [] }, '', { data: { points: 12 } })
  expect(html).toContain('window.data = {"points":12}')
  expect(html).toContain('window.onData')
  expect(html).toContain("window.agentState = 'idle'")
  expect(buildArtifactDocument({ title: 'status', code: '<p>status</p>', loadingMessages: [] }, '', { data: {}, agentState: 'asking' })).toContain("window.agentState = 'asking'")
  expect(html).not.toContain('jaz:canvas-wheel')
})

test("a plugin's view gets what it saved for the thread only where a save is kept", () => {
  const input = { title: 'status', code: '<p>status</p>', loadingMessages: [] }
  const html = buildArtifactDocument(input, '', { data: {}, viewState: { answers: [[3, '</script>']] } })
  expect(html).toContain('window.viewState = {"answers":[[3,"\\u003c/script>"]]}')
  expect(html.match(/<\/script>/g)).toHaveLength(1)
  expect(buildArtifactDocument(input, '', { data: {}, viewState: null })).toContain('window.viewState = null')
  expect(buildArtifactDocument(input, '', { data: {} })).not.toContain('setViewState')
})

test('host CSP is always prepended even when plugin HTML mentions content-security-policy', () => {
  const html = buildArtifactDocument(
    {
      title: 'spoof',
      code: '<!doctype html><html><head><meta http-equiv="Content-Security-Policy" content="default-src *"></head><body>hi</body></html>',
      loadingMessages: [],
    },
    '',
  )
  const hostMeta = `<meta http-equiv="Content-Security-Policy" content="${buildArtifactCSP(false)}">`
  expect(html).toContain(hostMeta)
  expect(html.indexOf(hostMeta)).toBeLessThan(html.indexOf('default-src *'))
  expect(html).not.toContain('worker-src blob:')
  expect(html).not.toContain("'wasm-unsafe-eval'")
})

test('plugin view CSP adds active asset origin, worker-src blob:, wasm-unsafe-eval, and resolveAsset bridge strictly for plugin views', () => {
  const pluginHTML = buildArtifactDocument(
    { title: 'viewer', code: '<!doctype html><html><head></head><body><canvas></canvas></body></html>', loadingMessages: [] },
    '',
    { data: {}, pluginView: true, assetOrigin: 'https://storage.googleapis.com/some/path', resolveAsset: true },
  )
  expect(pluginHTML).toContain("script-src 'unsafe-inline' 'wasm-unsafe-eval'")
  expect(pluginHTML).toContain('worker-src blob:')
  expect(pluginHTML).toContain('connect-src https://cdnjs.cloudflare.com https://esm.sh https://cdn.jsdelivr.net https://unpkg.com https://storage.googleapis.com')
  expect(pluginHTML).toContain('img-src data: blob: https://cdnjs.cloudflare.com https://cdn.jsdelivr.net https://unpkg.com https://fonts.gstatic.com https://fonts.googleapis.com https://storage.googleapis.com')
  expect(pluginHTML).toContain('media-src data: blob: https://cdnjs.cloudflare.com https://cdn.jsdelivr.net https://unpkg.com https://storage.googleapis.com')
  expect(pluginHTML).toContain('jaz:artifact-resolve-asset')
  expect(pluginHTML).toContain('window.jaz = Object.assign(window.jaz || {}, { resolveAsset })')

  expect(sanitizeAssetOrigin('jaz-local://backend/v1/sessions/s1/assets/a.glb')).toBe('jaz-local://backend')
  expect(sanitizeAssetOrigin('http://127.0.0.1:5299/v1/sessions/s1/assets/a.glb')).toBe('http://127.0.0.1:5299')
  expect(sanitizeAssetOrigin('http://evil.example')).toBe('')
  expect(sanitizeAssetOrigin('javascript:alert(1)')).toBe('')

  const inlineHTML = buildArtifactDocument({ title: 'chart', code: '<p>chart</p>', loadingMessages: [] }, '')
  expect(inlineHTML).not.toContain('worker-src blob:')
  expect(inlineHTML).not.toContain("'wasm-unsafe-eval'")
  expect(inlineHTML).not.toContain('jaz:artifact-resolve-asset')
})

test('resolvePluginAssetRelpath resolves relative to view.file and blocks workspace escapes', () => {
  expect(resolvePluginAssetRelpath('outputs/assembly.glb', 'status.json')).toBe('outputs/assembly.glb')
  expect(resolvePluginAssetRelpath('assembly.glb', 'runs/scene.json')).toBe('runs/assembly.glb')
  expect(resolvePluginAssetRelpath('./meshes/part.glb', 'runs/scene.json')).toBe('runs/meshes/part.glb')
  expect(resolvePluginAssetRelpath('../outputs/assembly.glb', 'runs/scene.json')).toBe('outputs/assembly.glb')

  for (const bad of ['', '   ', '.', '..', '../secret.txt', '/etc/passwd', 'file:///etc/passwd', 'https://example.com/a.glb', 'a\\b.glb', 'a.glb?x=1', 'a.glb#frag']) {
    expect(() => resolvePluginAssetRelpath(bad, 'status.json')).toThrow()
  }
  expect(() => resolvePluginAssetRelpath('../../etc/passwd', 'runs/scene.json')).toThrow()
})
