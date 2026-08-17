import { expect, test } from 'bun:test'

test('new session history renders the durable first prompt while loading', async () => {
  const worker = new globalThis.Worker(new globalThis.URL('./PendingSessionHistory.worker.mjs', import.meta.url).href)
  try {
    const result = await new Promise((resolve, reject) => {
      worker.onmessage = (event) => resolve(event.data)
      worker.onerror = reject
    })
    expect(result).toEqual({ showsPrompt: true, showsSkeleton: false })
  } finally {
    worker.terminate()
  }
})
