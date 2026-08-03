import { describe, expect, test } from 'bun:test'
import { readSessionMessageStream } from './streamProtocol'

function streamResponse(...events) {
  const body = events.map((event) => `data: ${JSON.stringify(event)}\n\n`).join('')
  return Promise.resolve(new globalThis.Response(body, { status: 200 }))
}

async function rejection(promise) {
  try {
    await promise
  } catch (error) {
    return error
  }
  throw new Error('expected promise to reject')
}

describe('readSessionMessageStream', () => {
  test('rejects acceptance when the server errors before durable acknowledgement', async () => {
    const stream = readSessionMessageStream(
      streamResponse({ type: 'error', error: 'message persistence failed' }, { type: 'done' }),
      () => {},
    )

    const [acceptanceError, finishedError] = await Promise.all([
      rejection(stream.accepted),
      rejection(stream.finished),
    ])
    expect(acceptanceError.message).toBe('message persistence failed')
    expect(finishedError.message).toBe('message persistence failed')
  })

  test('keeps acceptance resolved when the agent fails afterward', async () => {
    const stream = readSessionMessageStream(
      streamResponse({ type: 'accepted' }, { type: 'error', error: 'agent failed' }, { type: 'done' }),
      () => {},
    )

    const finishedError = await rejection(stream.finished)
    await stream.accepted
    expect(finishedError.message).toBe('agent failed')
  })

  test('rejects a stream that closes without acknowledging the message', async () => {
    const stream = readSessionMessageStream(streamResponse({ type: 'done' }), () => {})

    const [acceptanceError, finishedError] = await Promise.all([
      rejection(stream.accepted),
      rejection(stream.finished),
    ])
    expect(acceptanceError.message).toBe('Message was not accepted by the server.')
    expect(finishedError.message).toBe('Message was not accepted by the server.')
  })
})
