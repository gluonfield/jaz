import { expect, test } from 'bun:test'
import { clockFace, clockOf, timerChange } from '@/components/canvas/timer'

test('a timer waits, runs to its end, pauses with what is left and resets', () => {
  const now = Date.parse('2026-11-13T10:00:00Z')
  const idle = clockOf({ id: 't', type: 'timer', seconds: 90 }, now)
  expect(idle).toEqual({ seconds: 90, left: 90, running: false })
  const started = timerChange(idle, 'start', now)
  expect(started).toEqual({ endsAt: '2026-11-13T10:01:30.000Z', left: null })
  const running = clockOf({ id: 't', type: 'timer', seconds: 90, endsAt: started.endsAt }, now + 30_400)
  expect(running).toEqual({ seconds: 90, left: 60, running: true })
  expect(timerChange(running, 'pause', now)).toEqual({ endsAt: null, left: 60 })
  expect(clockOf({ id: 't', type: 'timer', seconds: 90, left: 60 }, now)).toEqual({ seconds: 90, left: 60, running: false })
  const done = clockOf({ id: 't', type: 'timer', seconds: 90, endsAt: started.endsAt }, now + 200_000)
  expect(done).toEqual({ seconds: 90, left: 0, running: false })
  expect(timerChange(done, 'start', now).endsAt).toBe('2026-11-13T10:01:30.000Z')
  expect(timerChange(running, 'reset', now)).toEqual({ endsAt: null, left: null })
  expect(timerChange(idle, 'longer', now)).toEqual({ seconds: 150, endsAt: null, left: null })
  expect(timerChange(idle, 'shorter', now).seconds).toBe(60)
  expect(clockOf({ id: 't', type: 'timer', seconds: -5, endsAt: 'soon', left: 'x' }, now)).toEqual({ seconds: 300, left: 300, running: false })
})

test('the face shows minutes and seconds, and hours when there are any', () => {
  expect(clockFace(0)).toBe('0:00')
  expect(clockFace(305)).toBe('5:05')
  expect(clockFace(3725)).toBe('1:02:05')
})
