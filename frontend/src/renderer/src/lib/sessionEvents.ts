import type { SessionEvent } from '@/lib/api/types'
import { planSurfaceKey } from '@/lib/planSurface'

export function sessionEventCoalesceKey(event: SessionEvent): string {
  const planKey = planSurfaceKey(event)
  if (planKey) return planKey
  if (event.type === 'acp' && event.acp?.id) {
    if (event.acp.tool_calls?.length) return `acp_tools:${event.acp.id}`
    if (event.acp.error) return `acp_error:${event.acp.id}`
    return `acp_status:${event.acp.id}`
  }
  if (event.type === 'acp_tool' && event.acp?.id && event.acp.tool_calls?.[0]?.id) {
    return `acp_tool:${event.acp.id}:${event.acp.tool_calls[0].id}`
  }
  if ((event.type === 'permission_request' || event.type === 'permission_response') && event.permission?.id) {
    return `${event.type}:${event.permission.id}`
  }
  return ''
}

export function mergeSessionEvent(prev: SessionEvent[], event: SessionEvent): SessionEvent[] {
  if (event.seq) {
    const seqIndex = prev.findIndex((item) => item.seq === event.seq && item.session_id === event.session_id)
    if (seqIndex !== -1) {
      const next = [...prev]
      next[seqIndex] = event
      return next
    }
  }
  const key = sessionEventCoalesceKey(event)
  if (!key) return [...prev, event]
  const index = prev.findIndex((item) => sessionEventCoalesceKey(item) === key)
  if (index === -1) return [...prev, event]
  const next = [...prev]
  next[index] = event
  return next
}

export function coalesceSessionEvents(events: SessionEvent[]): SessionEvent[] {
  const bySeq = new Map<string, number>()
  const deduped: SessionEvent[] = []
  for (const event of events) {
    if (!event.seq) {
      deduped.push(event)
      continue
    }
    const seqKey = `${event.session_id}:${event.seq}`
    const existing = bySeq.get(seqKey)
    if (existing === undefined) {
      bySeq.set(seqKey, deduped.length)
      deduped.push(event)
    } else {
      deduped[existing] = event
    }
  }
  const byKey = new Map<string, number>()
  const indexed: { event: SessionEvent; index: number }[] = []
  deduped.forEach((event, sourceIndex) => {
    const key = sessionEventCoalesceKey(event)
    const slot = key ? byKey.get(key) : undefined
    if (slot === undefined) {
      if (key) byKey.set(key, indexed.length)
      indexed.push({ event, index: sourceIndex })
    } else {
      indexed[slot] = { event, index: indexed[slot].index }
    }
  })
  return indexed
    .sort((a, b) => {
      const seqA = a.event.seq ?? 0
      const seqB = b.event.seq ?? 0
      if (seqA && seqB && a.event.session_id === b.event.session_id) {
        return seqA - seqB
      }
      const atA = Date.parse(a.event.at)
      const atB = Date.parse(b.event.at)
      const timeA = Number.isNaN(atA) ? 0 : atA
      const timeB = Number.isNaN(atB) ? 0 : atB
      return timeA - timeB || seqA - seqB || a.index - b.index
    })
    .map((item) => item.event)
}
