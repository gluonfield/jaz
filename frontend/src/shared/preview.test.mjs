import { describe, expect, test } from 'bun:test'
import { shouldProxyPreview } from './preview'

describe('shouldProxyPreview', () => {
  test('proxies server loopback targets but not generated preview origins', () => {
    expect(shouldProxyPreview('http://localhost:3000/')).toBeTrue()
    expect(shouldProxyPreview('http://127.0.0.1:3000/')).toBeTrue()
    expect(shouldProxyPreview('http://jaz-preview-capability.localhost:5299/')).toBeFalse()
    expect(shouldProxyPreview('https://preview.example.test/')).toBeFalse()
  })
})
