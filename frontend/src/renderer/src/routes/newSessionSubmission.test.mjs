import { expect, test } from 'bun:test'

async function submission(parameters = '') {
  const worker = new globalThis.Worker(new globalThis.URL('./newSessionSubmission.worker.mjs', import.meta.url).href)
  try {
    return await new Promise((resolve, reject) => {
      worker.onmessage = (event) => resolve(event.data)
      worker.onerror = reject
      worker.postMessage(parameters)
    })
  } finally {
    worker.terminate()
  }
}

test('opens with the first prompt visible only after persisting it once', async () => {
  const result = await submission()

  expect(result).toEqual({
    beforeAcknowledgement: { route: '/new', settled: false },
    route: '/sessions/session-1',
    initialPrompt: {
      sessionId: 'session-1',
      baselineMessageSeq: 0,
      message: {
        role: 'user',
        content: 'inspect this',
        blocks: [
          { type: 'quote', text: 'evidence', comment: 'note' },
          { type: 'text', text: 'inspect this' },
          { type: 'attachment', id: 'attachment-1', name: 'evidence.txt' },
        ],
        validTimestamp: true,
      },
    },
    projectedBeforeHistory: [[1_000_000, 'inspect this']],
    projectedAfterHistory: [[1, 'inspect this']],
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
})

test('applies the new composer choice before submitting its first prompt', async () => {
  for (const fastMode of ['on', 'off']) {
    const result = await submission(`?fast=${fastMode}`)
    expect(result.beforeConfigAcknowledgement).toEqual({
      route: '/new',
      settled: false,
      requests: [
        { path: '/v1/sessions', body: { title: 'Inspect this' } },
        { path: '/v1/sessions/session-1/agent/config', body: { id: 'fast-mode', value: fastMode } },
      ],
    })
    expect(result.requests.map((request) => request.path)).toEqual([
      '/v1/sessions', '/v1/sessions/session-1/agent/config', '/v1/sessions/session-1/queue',
    ])
    expect(result.route).toBe('/sessions/session-1')
  }
})

test('a rejected initial setting prevents the first prompt and navigation', async () => {
  const result = await submission('?fast=on&reject=1')
  expect(result.route).toBe('/new')
  expect(result.settled).toBe(false)
  expect(result.error).toBe('Fast Mode is unavailable')
  expect(result.requests.map((request) => request.path)).toEqual([
    '/v1/sessions', '/v1/sessions/session-1/agent/config',
  ])
})
