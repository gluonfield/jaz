import { describe, expect, test } from 'bun:test'
import { normalizePreviewURL, shouldProxyPreview } from './preview'
import { resolveFileLink } from './fileReader'

test('browser input preserves local paths and file URIs', () => {
  for (const path of ['/Users/wins/.jaz/workspaces/default/manufacturing/research/qdd-actuator-2026-09-17/index.html', 'file:///tmp/report%20one.html', 'C:\\reports\\index.html']) {
    expect(normalizePreviewURL(path)).toBe(path)
  }
  expect(normalizePreviewURL('localhost:3000')).toBe('http://localhost:3000/')
  expect(normalizePreviewURL('example.com')).toBe('https://example.com/')
})

test('browser address input distinguishes searches from website addresses', () => {
  for (const input of ['weather', '  metal prices  ', 'café 東京 😀', 'a  b + c&d?e=f#tag/%', 'site:example.com "sheet metal"', '100% copper', 'C++', 'javascript:alert(1)']) {
    const url = new URL(normalizePreviewURL(input))
    expect(url.origin + url.pathname).toBe('https://www.google.com/search')
    expect([...url.searchParams]).toEqual([['q', input.trim()]])
    expect(url.hash).toBe('')
  }
  for (const [input, expected] of [
    ['  https://example.com/path?q=a%20b&x=1#part  ', 'https://example.com/path?q=a%20b&x=1#part'],
    ['HTTP://EXAMPLE.COM:8080/path', 'http://example.com:8080/path'],
    ['example.com/path?q=one+two#part', 'https://example.com/path?q=one+two#part'],
    ['münich.de', 'https://xn--mnich-kva.de/'],
    ['example。com', 'https://example.com/'],
    ['localhost', 'http://localhost/'],
    ['localhost:3000/app', 'http://localhost:3000/app'],
    ['127.0.0.1:3000', 'http://127.0.0.1:3000/'],
    ['2130706433', 'http://127.0.0.1/'],
    ['0.0.0.0:8080', 'http://0.0.0.0:8080/'],
    ['192.168.1.1:8080', 'https://192.168.1.1:8080/'],
    ['[::1]:3000', 'http://[::1]:3000/'],
    ['[2001:db8::1]:8080', 'https://[2001:db8::1]:8080/'],
    ['printer:8080/path', 'https://printer:8080/path'],
    ['printer:443/path', 'https://printer/path'],
    ['http://intranet', 'http://intranet/'],
    ['', ''],
    [' \t\n ', ''],
  ]) {
    expect(normalizePreviewURL(input)).toBe(expected)
  }
})

test('document links resolve relative files and preserve line references', () => {
  expect(resolveFileLink('bom.csv', '/work/report/index.md')).toEqual({ path: '/work/report/bom.csv' })
  expect(resolveFileLink('../index.html', '/work/report/notes.md')).toEqual({ path: '/work/report/../index.html' })
  expect(resolveFileLink('app.ts:12', '/work/report/notes.md')).toEqual({ path: '/work/report/app.ts', line: 12 })
  for (const url of ['https://example.com/report.html', 'javascript:alert(1)', '#heading']) {
    expect(resolveFileLink(url, '/work/notes.md')).toBeNull()
  }
})

describe('shouldProxyPreview', () => {
  test('proxies server loopback targets but not generated preview origins', () => {
    expect(shouldProxyPreview('http://localhost:3000/')).toBeTrue()
    expect(shouldProxyPreview('http://127.0.0.1:3000/')).toBeTrue()
    expect(shouldProxyPreview('http://jaz-preview-capability.localhost:5299/')).toBeFalse()
    expect(shouldProxyPreview('https://preview.example.test/')).toBeFalse()
  })
})
