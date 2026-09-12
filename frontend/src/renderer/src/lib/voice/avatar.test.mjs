import { expect, test } from 'bun:test'
import { VoiceAvatarActivity } from './avatar'

const ready = { phase: 'listening', muted: false, speakerMuted: false, activity: null, error: '' }

test('microphone input never becomes assistant speech; short output pauses preserve the speaking shape', () => {
  const meter = new VoiceAvatarActivity()
  expect(meter.sample(ready, 0.6, 0, 1000)).toBe('listening')
  expect(meter.sample(ready, 0.6, 0.5, 1100)).toBe('speaking')
  expect(meter.sample(ready, 0, 0, 1350)).toBe('speaking')
  expect(meter.sample(ready, 0, 0, 1600)).toBe('listening')
})

test('conversation audio temporarily takes precedence over background work and returns to its latest activity', () => {
  const meter = new VoiceAvatarActivity()
  const thinking = { ...ready, activity: 'thinking' }
  const working = { ...ready, activity: 'working' }
  expect(meter.sample(thinking, 0, 0, 1000)).toBe('thinking')
  expect(meter.sample(working, 0, 0, 1100)).toBe('working')
  expect(meter.sample(working, 0.6, 0, 1200)).toBe('listening')
  expect(meter.sample(thinking, 0, 0, 1650)).toBe('thinking')
  expect(meter.sample(thinking, 0, 0.5, 1700)).toBe('speaking')
  expect(meter.sample(working, 0, 0, 2100)).toBe('working')
})

test('microphone mute preserves agent work and output; closing clears audio activity before reconnecting', () => {
  const meter = new VoiceAvatarActivity()
  const muted = { ...ready, muted: true }
  expect(meter.sample(muted, 0.6, 0, 1000)).toBe('muted')
  expect(meter.sample({ ...muted, activity: 'working' }, 0.6, 0, 1050)).toBe('working')
  expect(meter.sample(muted, 0.6, 0.5, 1100)).toBe('speaking')
  expect(meter.sample({ ...muted, phase: 'ending' }, 0.6, 0.5, 1150)).toBe('muted')
  expect(meter.sample({ ...ready, phase: 'connecting' }, 0, 0, 1200)).toBe('connecting')
  expect(meter.sample(ready, 0, 0, 1250)).toBe('listening')
  expect(meter.sample({ ...ready, phase: 'error' }, 0, 0, 1300)).toBe('error')
})
