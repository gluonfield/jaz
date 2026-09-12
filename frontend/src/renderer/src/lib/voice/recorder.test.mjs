import { expect, test } from 'bun:test'
import { decodeVoiceEvent } from './protocol'
import { VoiceRecorder } from './recorder'

test('native turn.done replaces streamed speech without duplicating overlapping speakers', () => {
  const recorder = new VoiceRecorder()
  const receive = (event) => recorder.accept(decodeVoiceEvent(JSON.stringify(event)))
  const user = receive({ type: 'input_transcript.added', item: { text: 'What files' } })
  const voice = receive({ type: 'output_transcript.added', item: { text: 'Let me ' } })
  const userDone = receive({ type: 'turn.done', turn: { role: 'user', transcript: 'What files are here?' } })
  const voiceDone = receive({ type: 'turn.done', turn: { role: 'assistant', transcript: 'Let me check.' } })
  expect(userDone).toEqual({ ...user, text: 'What files are here?' })
  expect(voiceDone).toEqual({ ...voice, text: 'Let me check.' })
  const next = receive({ type: 'input_transcript.added', item: { text: 'And disk size?' } })
  expect(next.id).not.toBe(user.id)
  expect(next.call_id).toBe(user.call_id)
})

test('public transcript deltas join within a speaker turn and keep the last partial on close', () => {
  const recorder = new VoiceRecorder()
  const first = recorder.accept({ type: 'transcript', role: 'user', text: 'Check ' })
  expect(recorder.accept({ type: 'transcript', role: 'user', text: 'files.' })).toEqual({ ...first, text: 'Check files.' })
  recorder.accept({ type: 'transcript', role: 'assistant', text: 'Checking.' })
  const next = recorder.accept({ type: 'transcript', role: 'user', text: 'Thanks.' })
  expect(next.id).not.toBe(first.id)
  expect(next.text).toBe('Thanks.')
})

test('public delegation includes conversation context for short corrections', () => {
  const conversation = new VoiceRecorder()
  conversation.accept({ type: 'transcript', role: 'user', text: 'Use Thursday.', at: 100 })
  conversation.accept({ type: 'transcript', role: 'assistant', text: 'This Thursday?', at: 300 })
  conversation.accept({ type: 'transcript', role: 'user', text: 'Yes.', at: 500 })
  const request = conversation.request({ type: 'delegation', id: 'id', at: 600 })
  expect(request.text).toBe('Yes.')
  expect(request.context).toContain('User: Use Thursday.')
  expect(request.context).toContain('Voice: This Thursday?')
  expect(request.context).toContain('User: Yes.')
})

test('a delegation excludes future transcript fragments and avoids replaying delivered context', () => {
  const conversation = new VoiceRecorder()
  expect(conversation.request({ type: 'delegation', id: 'empty', at: 10 })).toBeUndefined()
  conversation.accept({ type: 'transcript', role: 'user', text: 'First request.', at: 100 })
  conversation.accept({ type: 'transcript', role: 'user', text: 'Future correction.', at: 300 })
  const first = conversation.request({ type: 'delegation', id: 'one', at: 200 })
  expect(first.text).toBe('First request.')
  expect(first.context).not.toContain('Future correction.')
  const next = conversation.request({ type: 'delegation', id: 'two', at: 400 })
  expect(next.text).toBe('Future correction.')
  expect(next.context).not.toContain('First request.')
})

test('delegation and visible speech use the same corrected native transcript', () => {
  const recorder = new VoiceRecorder()
  recorder.accept({ type: 'transcript', role: 'user', text: 'Delete the files', turnBased: true })
  const final = recorder.accept({ type: 'transcript_done', role: 'user', text: 'List the files.' })
  const request = recorder.request({ type: 'delegation', id: 'request' })
  expect(request.text).toBe(final.text)
  expect(request.context).toBe('User: List the files.')
})

test('a late native final does not replay the already delegated spoken instruction', () => {
  const recorder = new VoiceRecorder()
  recorder.accept({ type: 'transcript', role: 'user', text: 'List files.', turnBased: true })
  expect(recorder.request({ type: 'delegation', id: 'first' }).text).toBe('List files.')
  recorder.accept({ type: 'transcript_done', role: 'user', text: 'List the files.' })
  expect(recorder.request({ type: 'delegation', id: 'second' })).toBeUndefined()
  recorder.accept({ type: 'transcript_done', role: 'user', text: 'Check disk size.' })
  expect(recorder.request({ type: 'delegation', id: 'third' }).context).toBe('User: Check disk size.')
})
