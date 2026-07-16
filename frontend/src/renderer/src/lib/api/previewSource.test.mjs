import { describe, expect, test } from 'bun:test'
import { preparePreviewProxySource } from './previewSource'

const proxy = {
  url: 'https://capability.preview.example/app',
}

describe('preparePreviewProxySource', () => {
  test('keeps local Electron previews without a probe', async () => {
    let fetched = false
    const local = { url: 'http://jaz-preview-capability.localhost:5299/app' }
    const source = await preparePreviewProxySource(local, async () => {
      fetched = true
      throw new Error('unexpected fetch')
    })

    expect(source).toBe(local.url)
    expect(fetched).toBeFalse()
  })

  test('selects the isolated source for browser and mobile after a valid probe', async () => {
    let init
    let requested
    const source = await preparePreviewProxySource(proxy, async (input, requestInit) => {
      requested = input.toString()
      init = requestInit
      return { status: 204, headers: { get: () => 'ready' } }
    })

    expect(source).toBe(proxy.url)
    expect(requested).toBe('https://capability.preview.example/.well-known/jaz-preview')
    expect(init.credentials).toBe('omit')
    expect(init.referrerPolicy).toBe('no-referrer')
  })

  test('reports missing or misrouted preview origins instead of loading the frame', async () => {
    await expect(
      preparePreviewProxySource(proxy, async () => ({ status: 404, headers: { get: () => null } })),
    ).rejects.toThrow('Check the server preview URL template and its DNS, TLS, and reverse-proxy routing.')
    await expect(preparePreviewProxySource({ url: 'file:///tmp/preview' })).rejects.toThrow(
      'Remote preview is unreachable',
    )
  })
})
