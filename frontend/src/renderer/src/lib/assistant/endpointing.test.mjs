import { expect, test } from 'bun:test'
import { DEFAULT_ENDPOINT_CONFIG, DictationEndpoint } from './endpointing'

test('stops after post-speech silence once speech onset is detected', () => {
  const endpoint = new DictationEndpoint(1000)
  expect(endpoint.onLevel(0.01, 1500)).toBe('continue')
  expect(endpoint.onLevel(0.04, 2000)).toBe('continue')
  expect(endpoint.onLevel(0.005, 3000)).toBe('continue')
  expect(endpoint.onLevel(0.005, 3599)).toBe('continue')
  expect(endpoint.onLevel(0.005, 3600)).toBe('stop')
})

test('transcript progress counts as speech and resets the silence window', () => {
  const endpoint = new DictationEndpoint(0)
  expect(endpoint.onTranscript('remind me', 1000)).toBe('continue')
  expect(endpoint.tick(2500)).toBe('continue')
  expect(endpoint.onTranscript('remind me tomorrow', 2550)).toBe('continue')
  expect(endpoint.tick(4000)).toBe('continue')
  expect(endpoint.tick(4150)).toBe('stop')
})

test('returns no_speech when nothing is heard before noSpeechMs', () => {
  const endpoint = new DictationEndpoint(100)
  expect(endpoint.onLevel(0.005, 7099)).toBe('continue')
  expect(endpoint.tick(7100)).toBe('no_speech')
})

test('caps recording at maxDurationMs even while speech continues', () => {
  const endpoint = new DictationEndpoint(0)
  expect(endpoint.onTranscript('long note', 1000)).toBe('continue')
  expect(endpoint.onLevel(0.1, DEFAULT_ENDPOINT_CONFIG.maxDurationMs - 1)).toBe('continue')
  expect(endpoint.onLevel(0.1, DEFAULT_ENDPOINT_CONFIG.maxDurationMs)).toBe('stop')
})
