import { expect, test } from 'bun:test'

test('opens with the first prompt visible only after persisting it once', async () => {
  const worker = new globalThis.Worker(new globalThis.URL('./newSessionSubmission.worker.mjs', import.meta.url).href)
  try {
    const result = await new Promise((resolve, reject) => {
      worker.onmessage = (event) => resolve(event.data)
      worker.onerror = reject
    })

    expect(result).toEqual({
      beforeAcknowledgement: { route: '/new', settled: false },
      route: '/sessions/session-1',
      initialPrompt: {
        sessionId: 'session-1',
        user: ' inspect this ',
        baselineMessageSeq: 0,
        planRequested: true,
        goalRequested: true,
        contexts: [{ id: 'selection-1', type: 'selection', text: ' evidence ', comment: ' note ' }],
        attachments: [{ id: 'attachment-1', name: 'evidence.txt' }],
        reasoning: '',
        assistant: '',
        tools: [],
        validTimestamp: true,
      },
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
