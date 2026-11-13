import type { CanvasItem } from '@/lib/api/canvas'

export interface Clock {
  seconds: number
  left: number
  running: boolean
}

// A timer counts down to `endsAt` while it runs, holds `left` while paused,
// and otherwise waits at its full `seconds`.
export function clockOf(item: CanvasItem, now: number): Clock {
  const seconds = typeof item.seconds === 'number' && item.seconds > 0 ? Math.round(item.seconds) : 300
  const end = typeof item.endsAt === 'string' ? Date.parse(item.endsAt) : NaN
  if (!Number.isNaN(end)) {
    const left = Math.max(0, Math.ceil((end - now) / 1000))
    return { seconds, left, running: left > 0 }
  }
  return { seconds, left: typeof item.left === 'number' && item.left >= 0 ? Math.round(item.left) : seconds, running: false }
}

export type TimerAct = 'start' | 'pause' | 'reset' | 'longer' | 'shorter'

export function timerChange({ seconds, left }: Clock, act: TimerAct, now: number): Partial<CanvasItem> {
  if (act === 'start') return { endsAt: new Date(now + (left || seconds) * 1000).toISOString(), left: null }
  if (act === 'pause') return { endsAt: null, left }
  if (act === 'reset') return { endsAt: null, left: null }
  return { seconds: Math.max(60, seconds + (act === 'longer' ? 60 : -60)), endsAt: null, left: null }
}

export function clockFace(left: number): string {
  const hours = Math.floor(left / 3600)
  const minutes = Math.floor(left / 60) % 60
  const seconds = String(left % 60).padStart(2, '0')
  return hours ? `${hours}:${String(minutes).padStart(2, '0')}:${seconds}` : `${minutes}:${seconds}`
}
