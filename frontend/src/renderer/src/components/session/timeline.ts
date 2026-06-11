// The transcript's timeline model: turns raw history (messages + coalesced
// events) into the ordered, filtered, grouped items the Transcript component
// renders. Pure data — no JSX — so the component can memoize one buildTimeline
// call per data change.
import type { ACPPermission, ACPToolCall, ChatMessage, SessionEvent } from '@/lib/api/types'
import { planSurfaceFromEvent, planSurfaceKey } from '@/lib/planSurface'
import { hasPermissionSurface, normalized } from './TranscriptUtils'

export type TimelineItem =
  | { kind: 'message'; message: ChatMessage; at: number }
  | { kind: 'event'; event: SessionEvent; eventIndex: number; at: number; showHeader: boolean }
  | { kind: 'tools'; calls: ACPToolCall[]; at: number; key: string }

export interface Turn {
  opener?: TimelineItem
  items: TimelineItem[]
}

export function isParentChildACPEvent(event: SessionEvent): boolean {
  return Boolean(
    event.acp?.parent_id &&
      event.acp.parent_id === event.session_id &&
      event.acp.id !== event.session_id,
  )
}

export function hasWorkingStatusSurface(event: SessionEvent): boolean {
  return Boolean(event.acp && normalized(event.acp.state) === 'running')
}

// A status event whose only surface is the "working on …" link.
function isWorkingLinkOnly(event: SessionEvent): boolean {
  return (
    event.type === 'acp' &&
    hasWorkingStatusSurface(event) &&
    !planSurfaceFromEvent(event) &&
    !event.content &&
    !event.acp?.thought &&
    !event.acp?.error &&
    !event.acp?.tool_calls?.length
  )
}

function hasVisibleACPSurface(event: SessionEvent): boolean {
  const acp = event.acp
  if (!acp) return false
  const hasPlan = Boolean(planSurfaceFromEvent(event))
  if (isParentChildACPEvent(event)) {
    return Boolean(
      event.content ||
        acp.thought ||
        acp.error ||
        hasPlan ||
        hasWorkingStatusSurface(event),
    )
  }
  return Boolean(
    event.content ||
      acp.thought ||
      acp.error ||
      acp.tool_calls?.length ||
      hasPlan ||
      hasWorkingStatusSurface(event),
  )
}

function itemTime(value: string | undefined): number {
  const parsed = Date.parse(value ?? '')
  return Number.isNaN(parsed) ? 0 : parsed
}

// Head-only merge of two ordered streams preserves each stream's internal
// order even when timestamps are unreliable.
function mergeTimeline(
  messages: ChatMessage[],
  events: { event: SessionEvent; index: number }[],
): TimelineItem[] {
  const out: TimelineItem[] = []
  let i = 0
  let j = 0
  while (i < messages.length || j < events.length) {
    const message = messages[i]
    const entry = events[j]
    if (!entry || (message && itemTime(message.created_at) <= itemTime(entry.event.at))) {
      out.push({ kind: 'message', message, at: itemTime(message.created_at) })
      i += 1
    } else {
      out.push({
        kind: 'event',
        event: entry.event,
        eventIndex: entry.index,
        at: itemTime(entry.event.at),
        showHeader: false,
      })
      j += 1
    }
  }
  return out
}

// Consecutive tool events collapse into one run: "Explored 2 files, ran 1 command".
function groupToolRuns(items: TimelineItem[]): TimelineItem[] {
  const out: TimelineItem[] = []
  for (const item of items) {
    const isToolEvent =
      item.kind === 'event' &&
      item.event.type === 'acp_tool' &&
      item.event.acp?.tool_calls?.length === 1
    if (!isToolEvent) {
      out.push(item)
      continue
    }
    const call = (item as Extract<TimelineItem, { kind: 'event' }>).event.acp!.tool_calls![0]
    const prev = out.at(-1)
    if (prev?.kind === 'tools') {
      const existing = prev.calls.findIndex((candidate) => candidate.id === call.id)
      if (existing === -1) prev.calls.push(call)
      else prev.calls[existing] = call
      prev.at = item.at
      continue
    }
    out.push({ kind: 'tools', calls: [call], at: item.at, key: `tools-${call.id}` })
  }
  return out
}

function markEventHeaders(items: TimelineItem[], sessionId?: string): void {
  let previousACP = ''
  for (const item of items) {
    if (item.kind !== 'event') {
      if (item.kind === 'message') previousACP = ''
      continue
    }
    const acp = item.event.acp
    if (!acp) {
      previousACP = ''
      continue
    }
    // Own-page events need no byline; a child agent's run is introduced once.
    item.showHeader = Boolean(sessionId && acp.id !== sessionId && acp.id !== previousACP)
    previousACP = acp.id
  }
}

function splitTurns(items: TimelineItem[]): Turn[] {
  const turns: Turn[] = []
  for (const item of items) {
    if (item.kind === 'message' && item.message.role === 'user') {
      turns.push({ opener: item, items: [] })
      continue
    }
    if (!turns.length) turns.push({ items: [] })
    turns.at(-1)!.items.push(item)
  }
  return turns
}

// Work that may fold under "Worked for Xs"; plans, pending questions, and errors stay out.
export function isCollapsibleWork(
  item: TimelineItem,
  pendingPermissionIds: Set<string>,
  latestPlanIndex: Map<string, number>,
): boolean {
  if (item.kind === 'tools') return true
  if (item.kind !== 'event') return false
  const event = item.event
  if (event.type === 'acp_thought') return true
  if (event.type === 'acp_message') return true
  if (event.type === 'permission_request') {
    return !pendingPermissionIds.has(event.permission?.id ?? '')
  }
  const planSurface = planSurfaceFromEvent(event)
  if (planSurface) {
    return event.acp ? latestPlanIndex.get(event.acp.id) !== item.eventIndex : false
  }
  if (event.type === 'acp') {
    if (event.acp?.error) return false
    return true
  }
  return false
}

// Everything between raw history and JSX that doesn't depend on render-only
// state (working, tail). The component memoizes one call per data change so
// parent renders and streaming flags don't rebuild the timeline.
export function buildTimeline(
  messages: ChatMessage[],
  events: SessionEvent[],
  sessionId: string | undefined,
  groupTurns: boolean,
) {
  const combinedEvents = combineSequentialACPText(events)
  const permissionResolutions = new Map<string, ACPPermission>()
  const latestPermissionRequest = new Map<string, number>()
  const latestPlanEvent = new Map<string, number>()
  const latestToolEvent = new Map<string, number>()
  combinedEvents.forEach((event, index) => {
    if (event.type === 'permission_request' && event.permission) {
      latestPermissionRequest.set(event.permission.id, index)
    }
    if (event.type === 'permission_response' && event.permission) {
      permissionResolutions.set(event.permission.id, event.permission)
    }
    const acp = event.acp
    if (acp && planSurfaceFromEvent(event)) {
      latestPlanEvent.set(acp.id, index)
    }
    if (event.type === 'acp' && acp?.tool_calls?.length) {
      latestToolEvent.set(acp.id, index)
    }
  })
  const renderedEvents = combinedEvents
    .map((event, index) => ({ event, index }))
    .filter(({ event, index }) => {
      if (event.type === 'permission_response' || event.type === 'assistant') return false
      if (event.type === 'permission_request' && !hasPermissionSurface(event.permission)) {
        return false
      }
      if (
        event.type === 'permission_request' &&
        event.permission?.id &&
        latestPermissionRequest.get(event.permission.id) !== index
      ) {
        return false
      }
      const acp = event.acp
      const planSurface = planSurfaceFromEvent(event)
      if (!acp) {
        if (planSurface) return true
        return Boolean(event.content || event.permission)
      }
      if (!hasVisibleACPSurface(event)) return false
      // This page's own running state has no link to render — drop the event
      // instead of leaving an empty row.
      if (isWorkingLinkOnly(event) && acp.id === sessionId) return false
      if (planSurface) {
        const isLatestPlan = latestPlanEvent.get(acp.id) === index
        if (!isLatestPlan && !event.content && !acp.error && !acp.tool_calls?.length) return false
      }
      if (event.type === 'acp' && acp.tool_calls?.length && latestToolEvent.get(acp.id) !== index) {
        return Boolean(event.content || acp.error || planSurface)
      }
      return true
    })

  const pendingPermissionIds = new Set<string>()
  for (const event of combinedEvents) {
    if (event.type === 'permission_request' && event.permission?.id) {
      pendingPermissionIds.add(event.permission.id)
    }
    if (event.type === 'permission_response' && event.permission?.id) {
      pendingPermissionIds.delete(event.permission.id)
    }
  }

  const visibleMessages = messages.filter((message) => message.role === 'user' || message.role === 'assistant')
  const merged = groupToolRuns(mergeTimeline(visibleMessages, renderedEvents))
  // Live state isn't history: pending questions and working status anchor at
  // the bottom; an answered question returns to its chronological spot.
  const isPendingCard = (item: TimelineItem) =>
    item.kind === 'event' &&
    item.event.type === 'permission_request' &&
    pendingPermissionIds.has(item.event.permission?.id ?? '')
  const isWorkingLink = (item: TimelineItem) =>
    item.kind === 'event' && isWorkingLinkOnly(item.event)
  const pendingCards = merged.filter(isPendingCard)
  const workingLinks = merged.filter(isWorkingLink)
  const chronological = merged.filter((item) => !isPendingCard(item) && !isWorkingLink(item))
  // "Working on …" next to a question that's waiting for the user is noise.
  const anchored = [...(pendingCards.length ? [] : workingLinks), ...pendingCards]
  markEventHeaders([...chronological, ...anchored], sessionId)
  return {
    chronological,
    anchored,
    turns: groupTurns ? splitTurns(chronological) : [],
    permissionResolutions,
    latestPlanEvent,
    pendingPermissionIds,
  }
}

// Coalesced events keep their latest copy whose seq changes per update; key by
// identity so streamed deltas patch in place instead of remounting.
export function stableEventKey(event: SessionEvent): string {
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
  return `${event.session_id}:${event.seq ?? 'live'}`
}

function combineSequentialACPText(events: SessionEvent[]): SessionEvent[] {
  const out: SessionEvent[] = []
  let lastSourceSeq = 0
  for (const event of events) {
    const prev = out.at(-1)
    // A seq gap means another event sat between these chunks; keep the boundary.
    const contiguous = !prev?.seq || !event.seq || event.seq === lastSourceSeq + 1
    lastSourceSeq = event.seq ?? 0
    if (canMergeACPText(prev, event) && contiguous) {
      const merged: SessionEvent = {
        ...prev!,
        content:
          event.type === 'acp_message'
            ? `${prev!.content ?? ''}${event.content ?? ''}`
            : prev!.content,
        acp: prev!.acp
          ? {
              ...prev!.acp,
              thought:
                event.type === 'acp_thought'
                  ? `${prev!.acp.thought ?? ''}${event.acp?.thought ?? ''}`
                  : prev!.acp.thought,
              state: event.acp?.state ?? prev!.acp.state,
              stop_reason: event.acp?.stop_reason ?? prev!.acp.stop_reason,
              error: event.acp?.error ?? prev!.acp.error,
            }
          : prev!.acp,
      }
      out[out.length - 1] = merged
      continue
    }
    out.push(event)
  }
  return out
}

function canMergeACPText(prev: SessionEvent | undefined, event: SessionEvent): boolean {
  if (!prev?.acp || !event.acp) return false
  if (prev.type !== event.type) return false
  if (event.type !== 'acp_message' && event.type !== 'acp_thought') return false
  return prev.acp.id === event.acp.id
}
