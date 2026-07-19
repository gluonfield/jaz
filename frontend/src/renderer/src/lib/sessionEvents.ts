import type { SessionEvent } from '@/lib/api/types'
import { mergeProviderSubagentEvent } from '@/lib/providerSubagents'
import { taskSurfaceFromEvent, taskSurfaceKey } from '@/lib/taskSurface'

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

// Stable identity for non-text events that update in place rather than
// appending: child/agent status rows, running tool calls, permissions, and loop
// cards. Text runs have their own coalesce key below.
export function inPlaceEventKey(event: SessionEvent): string | null {
  if (event.type === 'acp' && event.acp?.id) {
    // The parent's view of a child is a single overview row that moves through
    // running → idle/failed/cancelled; collapse every state onto one key so a
    // failure (which carries an error) updates the row instead of forking it.
    if (isParentChildACPEvent(event)) return `acp_status:${event.acp.id}`
    if (event.acp.tool_calls?.length) return `acp_tools:${event.acp.id}`
    if (event.acp.error) return `acp_error:${event.acp.id}`
    return `acp_status:${event.acp.id}`
  }
  if (event.type === 'acp_tool' && event.acp?.id && event.acp.tool_calls?.[0]?.id) {
    return `acp_tool:${event.acp.id}:${event.acp.tool_calls[0].id}`
  }
  if (event.type === 'provider_subagent' && event.provider_subagent?.id) {
    return `provider_subagent:${event.provider_subagent.provider ?? ''}:${event.provider_subagent.id}`
  }
  if ((event.type === 'permission_request' || event.type === 'permission_response') && event.permission?.id) {
    return `${event.type}:${event.permission.id}`
  }
  if ((event.type === 'goal_update' && event.goal) || event.type === 'goal_clear') {
    return `goal_update:${event.session_id}`
  }
  if (event.type === 'loop_created' && event.loop_created?.loop_id) {
    return `loop_created:${event.loop_created.loop_id}`
  }
  return null
}

export function sessionEventCoalesceKey(event: SessionEvent): string {
  return textRunEventKey(event) || taskSurfaceKey(event) || inPlaceEventKey(event) || ''
}

function replaceableEventKey(event: SessionEvent): string {
  return taskSurfaceKey(event) || inPlaceEventKey(event) || ''
}

export function mergeSessionEvent(prev: SessionEvent[], event: SessionEvent): SessionEvent[] {
  if (event.seq) {
    const seqIndex = prev.findIndex((item) => item.seq === event.seq && item.session_id === event.session_id)
    if (seqIndex !== -1) {
      const next = [...prev]
      next[seqIndex] = preferredDuplicateEvent(next[seqIndex], event)
      return next
    }
  }
  const textIndex = findOpenACPTextIndex(prev, event)
  if (textIndex !== -1) {
    const next = [...prev]
    next[textIndex] = mergeACPTextEvents(next[textIndex], event)!
    return next
  }
  const key = replaceableEventKey(event)
  if (!key) return [...prev, event]
  const index = prev.findIndex((item) => replaceableEventKey(item) === key)
  if (index === -1) return [...prev, event]
  const next = [...prev]
  next[index] = mergeCoalescedSessionEvent(next[index], event)
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
      deduped[existing] = preferredDuplicateEvent(deduped[existing], event)
    }
  }
  const byKey = new Map<string, number>()
  const indexed: { event: SessionEvent; index: number }[] = []
  compactACPTextEvents(deduped).forEach((event, sourceIndex) => {
    const key = replaceableEventKey(event)
    const slot = key ? byKey.get(key) : undefined
    if (slot === undefined) {
      if (key) byKey.set(key, indexed.length)
      indexed.push({ event, index: sourceIndex })
    } else {
      indexed[slot] = { event: mergeCoalescedSessionEvent(indexed[slot].event, event), index: indexed[slot].index }
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

function mergeCoalescedSessionEvent(prev: SessionEvent, next: SessionEvent): SessionEvent {
  if (prev.type === 'provider_subagent' && next.type === 'provider_subagent' && next.provider_subagent) {
    return {
      ...next,
      provider_subagent: mergeProviderSubagentEvent(prev.provider_subagent, next.provider_subagent),
    }
  }
  return next
}

function compactACPTextEvents(events: SessionEvent[]): SessionEvent[] {
  const compacted: SessionEvent[] = []
  let openTextIndex = -1
  for (const event of events) {
    if (isACPTextEvent(event)) {
      const merged = openTextIndex === -1 ? undefined : mergeACPTextEvents(compacted[openTextIndex], event)
      if (merged) {
        compacted[openTextIndex] = merged
      } else {
        compacted.push(event)
        openTextIndex = compacted.length - 1
      }
      continue
    }
    compacted.push(event)
    if (openTextIndex !== -1 && !keepsACPTextOpen(compacted[openTextIndex], event)) {
      openTextIndex = -1
    }
  }
  return compacted
}

function findOpenACPTextIndex(events: SessionEvent[], next: SessionEvent): number {
  if (!isACPTextEvent(next)) return -1
  for (let index = events.length - 1; index >= 0; index -= 1) {
    const event = events[index]
    if (isACPTextEvent(event)) return canMergeACPText(event, next) ? index : -1
    if (!keepsACPTextOpen(next, event)) return -1
  }
  return -1
}

export function mergeACPTextEvents(prev: SessionEvent | undefined, event: SessionEvent): SessionEvent | undefined {
  if (!prev || !canMergeACPText(prev, event)) return undefined
  const acp = prev.acp
    ? {
        ...prev.acp,
        state: event.acp?.state ?? prev.acp.state,
        stop_reason: event.acp?.stop_reason ?? prev.acp.stop_reason,
        error: event.acp?.error ?? prev.acp.error,
      }
    : prev.acp
  return {
    ...prev,
    seq: event.seq ?? prev.seq,
    at: event.at || prev.at,
    content:
      event.type === 'acp_message'
        ? `${prev.content ?? ''}${event.content ?? ''}`
        : prev.content,
    acp: acp
      ? {
          ...acp,
          thought:
            event.type === 'acp_thought'
              ? `${prev.acp?.thought ?? ''}${event.acp?.thought ?? ''}`
              : acp.thought,
        }
      : acp,
  }
}

function canMergeACPText(prev: SessionEvent | undefined, event: SessionEvent): boolean {
  if (!prev?.acp || !event.acp) return false
  if (prev.type !== event.type) return false
  if (event.type !== 'acp_message' && event.type !== 'acp_thought') return false
  if (prev.session_id !== event.session_id || prev.acp.id !== event.acp.id) return false
  return Boolean(prev.acp.text_run_id && prev.acp.text_run_id === event.acp.text_run_id)
}

function isACPTextEvent(event: SessionEvent): boolean {
  return Boolean(event.acp && (event.type === 'acp_message' || event.type === 'acp_thought'))
}

function keepsACPTextOpen(text: SessionEvent, event: SessionEvent): boolean {
  if (!text.acp || text.session_id !== event.session_id) return false
  if (event.acp?.id === text.acp.id) {
    if (event.type === 'acp_tool') return true
    if (event.type === 'acp') return acpStatusKeepsTextOpen(event)
  }
  return event.type === 'provider_subagent' && event.provider_subagent?.parent_id === text.acp.id
}

function acpStatusKeepsTextOpen(event: SessionEvent): boolean {
  const acp = event.acp
  if (!acp || acp.error) return false
  if (acp.assistant || acp.thought || acp.plan?.length || acp.tool_calls?.length || acp.permissions?.length || acp.goal_requested) {
    return false
  }
  return !acp.state || acp.state === 'starting' || acp.state === 'running'
}

function preferredDuplicateEvent(existing: SessionEvent, incoming: SessionEvent): SessionEvent {
  if (!sameACPTextEvent(existing, incoming)) return incoming
  return acpTextLength(existing) >= acpTextLength(incoming) ? existing : incoming
}

function sameACPTextEvent(a: SessionEvent, b: SessionEvent): boolean {
  return Boolean(
    a.type === b.type &&
      (a.type === 'acp_message' || a.type === 'acp_thought') &&
      a.acp?.id &&
      a.acp.id === b.acp?.id,
  )
}

function textRunEventKey(event: SessionEvent): string | null {
  if (
    (event.type === 'acp_message' || event.type === 'acp_thought') &&
    event.acp?.id &&
    event.acp.text_run_id
  ) {
    return `acp_text:${event.session_id}:${event.acp.id}:${event.type}:${event.acp.text_run_id}`
  }
  return null
}

function acpTextLength(event: SessionEvent): number {
  if (event.type === 'acp_message') return event.content?.length ?? 0
  if (event.type === 'acp_thought') return event.acp?.thought?.length ?? 0
  return 0
}
