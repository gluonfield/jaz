import { expect, test } from 'bun:test'
import { ApiError, readAPIResponse } from './response'

const { Response } = globalThis

test('an empty 204 acknowledges a completed write', async () => {
  await expect(readAPIResponse(new Response(null, { status: 204 }))).resolves.toBeUndefined()
})

test('JSON acknowledgements and server errors retain their existing contracts', async () => {
  await expect(readAPIResponse(Response.json({ accepted: true }))).resolves.toEqual({ accepted: true })
  await expect(readAPIResponse(Response.json({ error: 'Could not save' }, { status: 503 }))).rejects.toMatchObject({ status: 503, message: 'Could not save' })
})

test('an empty 200 response still exposes a broken JSON contract', async () => {
  await expect(readAPIResponse(new Response('', { status: 200 }))).rejects.toBeInstanceOf(SyntaxError)
})

test('plain-text errors preserve HTTP status and the shared error type', async () => {
  const response = new Response('Proxy unavailable', { status: 502, statusText: 'Bad Gateway' })
  const result = readAPIResponse(response)
  await expect(result).rejects.toBeInstanceOf(ApiError)
  await expect(result).rejects.toMatchObject({ status: 502, message: '502 Bad Gateway' })
})
