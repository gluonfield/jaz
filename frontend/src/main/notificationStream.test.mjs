import { expect, test } from 'bun:test'
import { createAlertStream } from './notificationStream'

const event = (key) => `data: ${JSON.stringify({ session_id: 't1', key, state: 'reply', kind: 'chat', title: 'T', body: 'B', at: '2026-11-01T22:00:00Z' })}\n\n`

function sse(chunks) {
  const encoder = new TextEncoder()
  return new ReadableStream({
    start(controller) {
      for (const chunk of chunks) controller.enqueue(encoder.encode(chunk))
      controller.close()
    },
  })
}

test('reads alerts split across chunks, skips heartbeats and stops on demand', async () => {
  const seen = []
  const requests = []
  const whole = event('completed:1')
  const stream = createAlertStream(
    (alert) => seen.push(alert.key),
    (url, init) => {
      requests.push({ url, authorization: init.headers.Authorization })
      if (requests.length > 1) return new Promise(() => {})
      return Promise.resolve(new Response(sse([': ping\n\n', whole.slice(0, 20), whole.slice(20), 'data: {"broken"\n\n', event('completed:2')])))
    },
  )
  stream.follow({ baseUrl: 'https://jaz.example', token: 'secret' })
  stream.follow({ baseUrl: 'https://jaz.example', token: 'secret' })
  await new Promise((resolve) => setTimeout(resolve, 20))
  expect(seen).toEqual(['completed:1', 'completed:2'])
  expect(requests).toEqual([{ url: 'https://jaz.example/v1/notifications/stream', authorization: 'Bearer secret' }])
  await new Promise((resolve) => setTimeout(resolve, 1100))
  expect(requests).toHaveLength(2)
  stream.stop()
})
