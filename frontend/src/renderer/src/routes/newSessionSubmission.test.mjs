import { expect, test } from 'bun:test'

test('opens the session only after persisting its first prompt once', async () => {
  const worker = new globalThis.Worker(new globalThis.URL('./newSessionSubmission.worker.mjs', import.meta.url).href)
  try {
    const result = await new Promise((resolve, reject) => {
      worker.onmessage = (event) => resolve(event.data)
      worker.onerror = reject
    })

    expect(result).toEqual({
      beforeAcknowledgement: { route: '/new', settled: false },
      route: '/sessions/session-1',
      requests: [
        { path: '/v1/sessions', body: { title: 'Inspect this' } },
        {
          path: '/v1/sessions/session-1/queue',
          body: {
            op: 'append',
            message: {
              text: 'inspect this',
              contexts: [{ type: 'selection', text: 'evidence', comment: 'note' }],
              attachment_ids: ['attachment-1'],
              plan_requested: true,
              goal_requested: true,
            },
          },
        },
      ],
    })
  } finally {
    worker.terminate()
  }
})
