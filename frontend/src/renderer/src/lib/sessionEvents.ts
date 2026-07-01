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

// Stable identity for the events that update in place rather than appending:
// a child/agent status row, a running tool call, a permission prompt, a loop
// card. Returns null for events that have no such identity (plain text deltas,
// unknown types). Shared by the live-merge key and the React render key so the
// two can never disagree on what counts as "the same row".
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
  return taskSurfaceKey(event) || inPlaceEventKey(event) || ''
}

export function mergeSessionEvent(prev: SessionEvent[], event: SessionEvent): SessionEvent[] {
  const base = withoutReplacedSessionEvents(prev, event)
  if (event.seq) {
    const seqIndex = base.findIndex((item) => item.seq === event.seq && item.session_id === event.session_id)
    if (seqIndex !== -1) {
      const next = [...base]
      next[seqIndex] = preferredDuplicateEvent(next[seqIndex], event)
      return next
    }
  }
  const merged = mergeAdjacentACPText(base.at(-1), event)
  if (merged) {
    const next = [...base]
    next[next.length - 1] = merged
    return next
  }
  const key = sessionEventCoalesceKey(event)
  if (!key) return [...base, event]
  const index = base.findIndex((item) => sessionEventCoalesceKey(item) === key)
  if (index === -1) return [...base, event]
  const next = [...base]
  next[index] = mergeCoalescedSessionEvent(next[index], event)
  return next
}

export function coalesceSessionEvents(events: SessionEvent[]): SessionEvent[] {
  const replaced = replacedSessionEventSeqs(events)
  const bySeq = new Map<string, number>()
  const deduped: SessionEvent[] = []
  for (const event of events) {
    if (event.seq && replaced.has(sessionEventSeqKey(event.session_id, event.seq))) {
      continue
    }
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
  deduped.forEach((event, sourceIndex) => {
    const key = sessionEventCoalesceKey(event)
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

function withoutReplacedSessionEvents(prev: SessionEvent[], event: SessionEvent): SessionEvent[] {
  if (!event.replace_seqs?.length) return prev
  const replaced = new Set(event.replace_seqs.map((seq) => sessionEventSeqKey(event.session_id, seq)))
  return prev.filter((item) => !item.seq || !replaced.has(sessionEventSeqKey(item.session_id, item.seq)))
}

function replacedSessionEventSeqs(events: SessionEvent[]): Set<string> {
  const replaced = new Set<string>()
  for (const event of events) {
    for (const seq of event.replace_seqs ?? []) {
      replaced.add(sessionEventSeqKey(event.session_id, seq))
    }
  }
  return replaced
}

function sessionEventSeqKey(sessionID: string, seq: number): string {
  return `${sessionID}:${seq}`
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

function mergeAdjacentACPText(prev: SessionEvent | undefined, event: SessionEvent): SessionEvent | undefined {
  if (!prev?.seq || !event.seq || event.seq === prev.seq + 1) {
    return mergeACPTextEvents(prev, event)
  }
  return undefined
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
  if (prev.acp.text_run_id || event.acp.text_run_id) {
    return Boolean(prev.acp.text_run_id && prev.acp.text_run_id === event.acp.text_run_id)
  }
  return true
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

function acpTextLength(event: SessionEvent): number {
  if (event.type === 'acp_message') return event.content?.length ?? 0
  if (event.type === 'acp_thought') return event.acp?.thought?.length ?? 0
  return 0
}
