import type { SessionEvent } from '@/lib/api/types'
import { taskSurfaceFromEvent } from '@/lib/taskSurface'

// A child agent's run as seen from its parent. Bare child status belongs in
// Overview > Threads; child task surfaces can still stay inline when actionable.
export function isParentChildACPEvent(event: SessionEvent): boolean {
  return Boolean(
    event.acp?.parent_id &&
      event.acp.parent_id === event.session_id &&
      event.acp.id !== event.session_id,
  )
}

export type SessionEventPlacement = 'transcript' | 'overview' | 'side_chat'

export function sessionEventPlacement(event: SessionEvent): SessionEventPlacement {
  switch (event.type) {
    case 'acp':
      return isParentChildACPEvent(event) && !taskSurfaceFromEvent(event) ? 'overview' : 'transcript'
    case 'provider_subagent':
      return 'overview'
    case 'goal_update':
    case 'goal_clear':
      return 'overview'
    case 'side_chat_message':
      return 'side_chat'
    default:
      return 'transcript'
  }
}

export function sessionEventCoalesceKey(event: SessionEvent): string {
  return event.projection_key ?? ''
}

export function mergeSessionEvent(prev: SessionEvent[], event: SessionEvent): SessionEvent[] {
  const key = sessionEventCoalesceKey(event)
  let keyIndex = -1
  for (let index = 0; index < prev.length; index += 1) {
    const candidate = prev[index]
    if (event.seq && candidate.session_id === event.session_id && candidate.seq === event.seq)
      return prev
    if (key && sessionEventCoalesceKey(candidate) === key) keyIndex = index
  }
  if (keyIndex === -1) return [...prev, event]
  if (event.seq && (prev[keyIndex].seq ?? 0) >= event.seq) return prev
  const next = [...prev]
  next[keyIndex] = event.projection_op === 'append' ? appendProjection(next[keyIndex], event) : event
  return next
}

export function coalesceSessionEvents(events: SessionEvent[]): SessionEvent[] {
  const ordered = events
    .map((event, index) => ({ event, index }))
    .sort((a, b) => compareEvents(a.event, b.event) || a.index - b.index)
  const projected: SessionEvent[] = []
  const seenSeq = new Set<string>()
  const byKey = new Map<string, number>()
  for (const { event } of ordered) {
    const seqKey = event.seq ? `${event.session_id}:${event.seq}` : ''
    if (seqKey && seenSeq.has(seqKey)) continue
    if (seqKey) seenSeq.add(seqKey)
    const key = sessionEventCoalesceKey(event)
    const index = key ? byKey.get(key) : undefined
    if (index === undefined) {
      if (key) byKey.set(key, projected.length)
      projected.push(event)
    } else {
      projected[index] = event.projection_op === 'append' ? appendProjection(projected[index], event) : event
    }
  }
  return projected.sort(compareEvents)
}

function appendProjection(prev: SessionEvent, event: SessionEvent): SessionEvent {
  return {
    ...prev,
    ...event,
    content:
      event.type === 'acp_message'
        ? `${prev.content ?? ''}${event.content ?? ''}`
        : (event.content ?? prev.content),
    acp: event.acp
      ? {
          ...prev.acp,
          ...event.acp,
          thought:
            event.type === 'acp_thought'
              ? `${prev.acp?.thought ?? ''}${event.acp.thought ?? ''}`
              : (event.acp.thought ?? prev.acp?.thought),
        }
      : event.acp,
  }
}

function compareEvents(a: SessionEvent, b: SessionEvent): number {
  const seqA = a.seq ?? 0
  const seqB = b.seq ?? 0
  if (seqA && seqB && a.session_id === b.session_id) return seqA - seqB
  const timeA = Date.parse(a.at)
  const timeB = Date.parse(b.at)
  return (Number.isNaN(timeA) ? 0 : timeA) - (Number.isNaN(timeB) ? 0 : timeB) || seqA - seqB
}
