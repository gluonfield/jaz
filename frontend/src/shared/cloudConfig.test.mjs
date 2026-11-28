import { expect, test } from 'bun:test'
import { normalizeCloudUrl, parseBoolFlag, readNodeCloudConfig, resolveCloudConfig } from './cloudConfig'

test('parseBoolFlag handles truthy, falsy, and fallback values', () => {
  expect(parseBoolFlag('false', true)).toBe(false)
  expect(parseBoolFlag('0', true)).toBe(false)
  expect(parseBoolFlag('OFF', true)).toBe(false)
  expect(parseBoolFlag('true', false)).toBe(true)
  expect(parseBoolFlag('1', false)).toBe(true)
  expect(parseBoolFlag('', true)).toBe(true)
  expect(parseBoolFlag(undefined, false)).toBe(false)
  expect(parseBoolFlag(false, true)).toBe(false)
})

test('normalizeCloudUrl accepts HTTPS and loopback origins and rejects insecure remote HTTP', () => {
  expect(normalizeCloudUrl('https://jaz.acme.com/')).toBe('https://jaz.acme.com')
  expect(normalizeCloudUrl('https://jaz.acme.com/some/path')).toBe('https://jaz.acme.com')
  expect(normalizeCloudUrl('http://127.0.0.1:8080')).toBe('http://127.0.0.1:8080')
  expect(normalizeCloudUrl('http://jaz.acme.com')).toBe('')
  expect(normalizeCloudUrl('not a url')).toBe('')
  expect(normalizeCloudUrl('')).toBe('')
})

test('resolveCloudConfig lets runtime env override build-time values', () => {
  expect(resolveCloudConfig({
    buildUrl: 'https://baked.example.com',
    buildLocalEnabled: 'false',
    defaultLocalEnabled: true,
  })).toEqual({
    cloudUrl: 'https://baked.example.com',
    localEnabled: false,
  })

  expect(resolveCloudConfig({
    runtimeUrl: 'https://override.example.com/',
    buildUrl: 'https://baked.example.com',
    runtimeLocalEnabled: 'true',
    buildLocalEnabled: 'false',
    defaultLocalEnabled: true,
  })).toEqual({
    cloudUrl: 'https://override.example.com',
    localEnabled: true,
  })
})

test('readNodeCloudConfig reads JAZ_CLOUD_URL and JAZ_LOCAL_ENABLED', () => {
  expect(readNodeCloudConfig({
    JAZ_CLOUD_URL: 'https://cloud.jaz.chat/',
    JAZ_LOCAL_ENABLED: 'false',
  })).toEqual({
    cloudUrl: 'https://cloud.jaz.chat',
    localEnabled: false,
  })
  expect(readNodeCloudConfig({})).toEqual({
    cloudUrl: '',
    localEnabled: true,
  })
})
