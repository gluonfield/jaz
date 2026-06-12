import type { SessionEvent } from '@/lib/api/types'

const LIVE_MS = 30_000
const STALE_MS = 120_000

export type RunSignal = 'idle' | 'live' | 'quiet' | 'stale'

export interface SessionRunSignal {
  signal: RunSignal
  ageMs?: number
}

function eventTime(iso: string | undefined): number {
  const time = Date.parse(iso ?? '')
  return Number.isFinite(time) && time > 0 ? time : 0
}

export function latestEventTimeISO(a: string | undefined, b: string | undefined): string | undefined {
  if (!a) return b
  if (!b) return a
  return eventTime(a) >= eventTime(b) ? a : b
}

function latestEventAt(events: SessionEvent[]): string | undefined {
  let lastEventAt: string | undefined
  for (const event of events) {
    lastEventAt = latestEventTimeISO(lastEventAt, event.at)
  }
  return lastEventAt
}

export function deriveSessionRunSignal({
  running,
  updatedAt,
  events,
  lastEventAt,
  now,
}: {
  running: boolean
  updatedAt: string
  events: SessionEvent[]
  lastEventAt?: string
  now: number
}): SessionRunSignal {
  if (!running) return { signal: 'idle' }

  const at = eventTime(latestEventTimeISO(latestEventAt(events), lastEventAt) ?? updatedAt)
  const ageMs = at ? Math.max(0, now - at) : undefined
  if (ageMs === undefined) return { signal: 'quiet' }
  if (ageMs <= LIVE_MS) return { signal: 'live', ageMs }
  if (ageMs <= STALE_MS) return { signal: 'quiet', ageMs }
  return { signal: 'stale', ageMs }
}
