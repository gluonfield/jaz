import { expect, test } from 'bun:test'

test('tracked popovers stay open and follow their anchor as the sidebar changes', async () => {
  const worker = new globalThis.Worker(new globalThis.URL('./PopoverTracking.worker.mjs', import.meta.url).href)
  try {
    const result = await new Promise((resolve, reject) => {
      worker.onmessage = (event) => resolve(event.data)
      worker.onerror = reject
    })
    expect(result).toEqual({
      openAfterScroll: true,
      followedScroll: true,
    })
  } finally {
    worker.terminate()
  }
})
