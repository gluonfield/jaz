import { describe, expect, test } from 'bun:test'
import { googlePasskeyDestination, normalizePreviewURL, shouldProxyPreview } from './preview'
import { findFileReferences, resolveFileLink } from './fileReader'

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
  expect(resolveFileLink('../index.html', '/work/report/notes.md')).toEqual({ path: '/work/index.html' })
  expect(resolveFileLink('app.ts:12', '/work/report/notes.md')).toEqual({ path: '/work/report/app.ts', location: { line: 12 } })
  for (const url of ['https://example.com/report.html', 'javascript:alert(1)', '#heading']) {
    expect(resolveFileLink(url, '/work/notes.md')).toBeNull()
  }
})

test('file links preserve typed PDF, workbook, text and line locations', () => {
  for (const [link, expected] of [
    ['file.pdf#page=34', { path: '/work/file.pdf', location: { page: 34 } }],
    ['file.pdf#page=34&text=liquidated%20damages', { path: '/work/file.pdf', location: { page: 34, text: 'liquidated damages' } }],
    ['tender/Commercial/BOQ.xlsx#Line level cost -DG!A21', { path: '/work/tender/Commercial/BOQ.xlsx', location: { sheet: 'Line level cost -DG', range: 'A21' } }],
    ['file.xlsx#Sheet%20name!B7:D9', { path: '/work/file.xlsx', location: { sheet: 'Sheet name', range: 'B7:D9' } }],
    ['file.xlsx#b7', { path: '/work/file.xlsx', location: { range: 'B7' } }],
    ['file.xlsx#Line%20level%20cost%20-DG', { path: '/work/file.xlsx', location: { sheet: 'Line level cost -DG' } }],
    ['notes.md#text=some%20words', { path: '/work/notes.md', location: { text: 'some words' } }],
    ['code.ts:27', { path: '/work/code.ts', location: { line: 27 } }],
    ['Makefile:27', { path: '/work/Makefile', location: { line: 27 } }],
    ['./tender/../notes.md#text=again', { path: '/work/notes.md', location: { text: 'again' } }],
  ]) expect(resolveFileLink(link, '/work/report.md')).toEqual(expected)
  expect(findFileReferences('See /work/file.pdf#page=34&text=some%20words.')[0].reference).toEqual({
    path: '/work/file.pdf', location: { page: 34, text: 'some words' },
  })
})

describe('shouldProxyPreview', () => {
  test('proxies server loopback targets but not generated preview origins', () => {
    expect(shouldProxyPreview('http://localhost:3000/')).toBeTrue()
    expect(shouldProxyPreview('http://127.0.0.1:3000/')).toBeTrue()
    expect(shouldProxyPreview('http://jaz-preview-capability.localhost:5299/')).toBeFalse()
    expect(shouldProxyPreview('https://preview.example.test/')).toBeFalse()
  })
})

describe('googlePasskeyDestination', () => {
  test('sanitizes Google passkey challenge URLs and ignores non-challenges or lookalikes', () => {
    expect(googlePasskeyDestination('https://accounts.google.com/v3/signin/challenge/pk/presend?TL=secret', 'Google Cloud Platform')).toBe('https://console.cloud.google.com/')
    expect(googlePasskeyDestination('https://accounts.google.com/v3/signin/challenge/pk/presend?TL=secret')).toBe('https://accounts.google.com/')
    expect(googlePasskeyDestination('https://accounts.google.com/signin/v2/challenge/pk?TL=secret')).toBe('https://accounts.google.com/')
    expect(googlePasskeyDestination('https://accounts.google.com/v3/signin/challenge/pwd/presend')).toBeNull()
    expect(googlePasskeyDestination('https://accounts.google.com.example.test/v3/signin/challenge/pk/presend')).toBeNull()
    expect(googlePasskeyDestination('http://accounts.google.com/v3/signin/challenge/pk/presend')).toBeNull()
  })
})
