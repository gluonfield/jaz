import { expect, test } from 'bun:test'
import { TextEncoder } from 'node:util'
import { decodeVoiceEvent, voiceContextEvents } from './protocol'

test('decodes subscription delegation content and public Live metadata separately', () => {
  expect(decodeVoiceEvent(JSON.stringify({ type: 'delegation.created', item: { id: 'opaque', type: 'delegation', target: 'client', content: [{ type: 'input_text', text: 'Check the files' }] } }))).toEqual({ type: 'delegation', id: 'opaque', text: 'Check the files' })
  expect(decodeVoiceEvent(JSON.stringify({ type: 'session.delegation.created', offset_ms: 1000, delegation: { id: 'opaque', type: 'delegation', target: 'client' } }))).toEqual({ type: 'delegation', id: 'opaque', at: 1000 })
  expect(decodeVoiceEvent(JSON.stringify({ type: 'session.delegation.created', delegation: { id: 'opaque', target: 'responses' } }))).toBeUndefined()
})

test('preserves Unicode and each provider’s result contract', () => {
  const text = 'Check the changed file 🗣️ Žodis. '.repeat(90)
  for (const provider of ['openai', 'openai-api-key']) {
    const events = voiceContextEvents(provider, text, true, 'exact-id')
    const chunks = events.map((event) => provider === 'openai' ? event.content[0].text : event.content)
    expect(chunks.join('')).toBe(text)
    expect(chunks.every((chunk) => new TextEncoder().encode(chunk).length <= 480)).toBe(true)
    expect(events.every((event) => provider === 'openai'
      ? event.type === 'delegation.context.append' && event.delegation_item_id === 'exact-id' && event.channel === 'speakable'
      : event.type === 'session.commentary.append' && event.delegation_id === 'exact-id')).toBe(true)
  }
})

test('progress uses quiet context; completion uses speech', () => {
  expect(voiceContextEvents('openai', 'Working', false, 'id')[0].channel).toBeUndefined()
  expect(voiceContextEvents('openai-api-key', 'Working', false, 'id')[0].type).toBe('session.thinking.append')
})
