import { expect, test } from 'bun:test'

test('a sent message stays on screen from send until the saved copy of it is loaded', async () => {
  const worker = new globalThis.Worker(new globalThis.URL('./messageDelivery.worker.mjs', import.meta.url).href)
  try {
    const steps = await new Promise((resolve, reject) => {
      worker.onmessage = (event) => resolve(event.data)
      worker.onerror = reject
    })
    expect(steps).toEqual({
      bothWaiting: ['ready', 'do the report (sending)', 'and email it (sending)'],
      bothSaved: ['ready', 'do the report', 'and email it'],
      outboxAfterSave: ['delivered', 'delivered'],
      acceptedWhileHistoryDown: ['ready', 'do the report', 'and email it', 'one more thing'],
      outboxWhileHistoryDown: ['delivered', 'delivered', 'delivered'],
      afterHistoryReturns: ['ready', 'do the report', 'and email it', 'one more thing'],
      settledAfterHistoryReturns: 3,
      refused: ['ready', 'do the report', 'and email it', 'one more thing', 'try this (failed)'],
      refusedError: 'agent did not start',
      savedAfterRefusal: 0,
      retried: ['ready', 'do the report', 'and email it', 'one more thing', 'try this'],
      savedAfterRetry: 1,
      stopped: 'stop me (failed)',
      cloudTurns: 1,
      cloudSaved: 1,
      queuedBehindRunningTurn: 'while you work',
      queuedTurnStarted: 1,
      queuedTurnSettled: true,
      cloudPendingBeforePersist: 'cloud async persist',
      cloudPersistedSettled: true,
      commandLeft: true,
    })
  } finally {
    worker.terminate()
  }
})
