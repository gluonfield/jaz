// The transcript's timeline model: turns raw history (messages + coalesced
// events) into the ordered, filtered, grouped items the Transcript component
// renders. Pure data — no JSX — so the component can memoize one buildTimeline
// call per data change.
import type { ACPPermission, ACPToolCall, ChatMessage, SessionEvent } from '@/lib/api/types'
import { taskSurfaceFromEvent } from '@/lib/taskSurface'
import { isParentChildACPEvent, sessionEventCoalesceKey } from '@/lib/sessionEvents'
import { hasPermissionSurface, normalized } from './TranscriptUtils'

export type TimelineItem =
  | { kind: 'message'; message: ChatMessage; at: number }
  | { kind: 'event'; event: SessionEvent; eventIndex: number; at: number; showHeader: boolean }
  | { kind: 'tools'; calls: ACPToolCall[]; at: number; key: string }

export interface Turn {
  opener?: TimelineItem
  items: TimelineItem[]
}

function hasWorkingStatusSurface(event: SessionEvent): boolean {
  return Boolean(event.acp && normalized(event.acp.state) === 'running')
}

// A running status whose only surface is the live "working" indicator — no
// content, thought, tools, error, or plan yet. It anchors to the bottom while
// live rather than sitting in the chronological flow.
function isWorkingStatusOnly(event: SessionEvent): boolean {
  return (
    event.type === 'acp' &&
    hasWorkingStatusSurface(event) &&
    !taskSurfaceFromEvent(event) &&
    !event.content &&
    !event.acp?.thought &&
    !event.acp?.error &&
    !event.acp?.tool_calls?.length
  )
}

function hasVisibleACPSurface(event: SessionEvent): boolean {
  const acp = event.acp
  if (!acp) return false
  const hasTaskSurface = Boolean(taskSurfaceFromEvent(event))
  if (isParentChildACPEvent(event)) {
    return event.type === 'acp' && hasTaskSurface
  }
  return Boolean(
    event.content ||
      acp.thought ||
      acp.error ||
      acp.tool_calls?.length ||
      hasTaskSurface ||
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

// True for mechanical work that folds under "Worked for Xs": tools, thinking,
// bare status, resolved permissions, superseded task surfaces. Assistant text is
// classified at turn scope by classifyTurnItems, not here.
export function isCollapsibleWork(
  item: TimelineItem,
  pendingPermissionIds: Set<string>,
  latestTaskSurfaceIndex: Map<string, number>,
): boolean {
  if (item.kind === 'tools') return true
  if (item.kind !== 'event') return false
  const event = item.event
  if (event.type === 'artifact') return false
  if (event.type === 'acp_thought') return true
  if (event.type === 'permission_request') {
    return !pendingPermissionIds.has(event.permission?.id ?? '')
  }
  const taskSurface = taskSurfaceFromEvent(event)
  if (taskSurface) {
    return event.acp ? latestTaskSurfaceIndex.get(event.acp.id) !== item.eventIndex : false
  }
  if (event.type === 'acp') {
    return true
  }
  return false
}

// An assistant text block: a streamed acp_message or an acp snapshot carrying
// answer text. Artifacts render their own surface and count as anchors, not text.
function textContent(item: TimelineItem): string | undefined {
  if (item.kind !== 'event' || item.event.type === 'artifact') return undefined
  return item.event.content?.trim() || undefined
}

// An item that reads as produced output rather than process. Result cards are
// pulled from the flow before classification, so artifacts are the only anchor.
function isOutcomeAnchor(item: TimelineItem): boolean {
  return item.kind === 'event' && item.event.type === 'artifact'
}

// Substance markers that separate an answer from a one-line preamble: a heading,
// a list, a table, display math, or more than one paragraph. The bias is toward
// showing — interim narration ("Let me check X first") is a single short
// paragraph and stays folded; anything richer reads as answer and shows.
function hasAnswerStructure(text: string): boolean {
  return (
    /(^|\n)\s{0,3}#{1,6}\s/.test(text) || // heading
    /(^|\n)\s*([-*+]|\d+\.)\s/.test(text) || // bullet or numbered list
    /(^|\n)\s*\|.*\|/.test(text) || // table row
    /\\\[|\$\$/.test(text) || // display math
    text.includes('\n\n') // multiple paragraphs
  )
}

const progressNarrationPattern =
  /\b(?:let me|i(?:'|’)ll|i will|i(?:'|’)m going to|i am going to|i(?:'|’)m (?:checking|running|opening|editing|installing|starting|staging|committing|doing|using|narrowing|tightening|changing))\b/i

function hasAnswerWeight(text: string): boolean {
  return text.length >= 400 || hasAnswerStructure(text)
}

function isLikelyProgressNarration(text: string): boolean {
  if (hasAnswerWeight(text)) return false
  return progressNarrationPattern.test(text)
}

function isSourceNote(text: string): boolean {
  return /^sources?:\s/i.test(text)
}

// Split a completed turn into the work that folds under one "Worked for Xs" and
// the result items shown inline, preserving order. With no final-answer marker
// in stored ACP text, terminal, artifact-linked, source-linked, structured, and
// substantial text stays visible; short unstructured non-terminal text folds as
// process narration. Classifying at turn scope — not per item — is what stops a
// shown message from splitting work into a staircase.
export function classifyTurnItems(
  flow: TimelineItem[],
  pendingPermissionIds: Set<string>,
  latestTaskSurfaceIndex: Map<string, number>,
): { workItems: TimelineItem[]; resultItems: TimelineItem[] } {
  // Pass 1: gather the turn-scope facts each text block needs. A text block
  // brackets an artifact when one sits between it and the adjacent text block.
  const artifactBracketed = new Set<number>()
  const sourceLinkedText = new Set<number>()
  const textIndices: number[] = []
  let lastTextIndex = -1
  let prevTextIndex = -1
  let artifactSincePrevText = false
  let hasAnchor = false
  let hasWork = false
  flow.forEach((item, index) => {
    const text = textContent(item)
    if (text !== undefined) {
      textIndices.push(index)
      if (isSourceNote(text)) {
        sourceLinkedText.add(index)
        if (prevTextIndex >= 0) sourceLinkedText.add(prevTextIndex)
      }
      if (artifactSincePrevText) {
        artifactBracketed.add(index)
        if (prevTextIndex >= 0) artifactBracketed.add(prevTextIndex)
      }
      prevTextIndex = index
      lastTextIndex = index
      artifactSincePrevText = false
    } else if (isOutcomeAnchor(item)) {
      hasAnchor = true
      artifactSincePrevText = true
    } else if (isCollapsibleWork(item, pendingPermissionIds, latestTaskSurfaceIndex)) {
      hasWork = true
    }
  })
  if (lastTextIndex >= 0 && isLikelyProgressNarration(textContent(flow[lastTextIndex]) ?? '')) {
    const previousText = textIndices.at(-2)
    if (previousText !== undefined) sourceLinkedText.add(previousText)
  }
  const showsText = (index: number): boolean =>
    (!hasWork && !hasAnchor) || // a tool-less turn is pure answer
    index === lastTextIndex || // terminal answer
    artifactBracketed.has(index) || // prose introducing/following an artifact
    sourceLinkedText.has(index) || // sources/trailing progress belong with the answer
    hasAnswerWeight(textContent(flow[index]) ?? '')

  // Pass 2: partition in place. Artifacts return false from isCollapsibleWork,
  // so they fall through to results alongside pending questions/task surfaces.
  const workItems: TimelineItem[] = []
  const resultItems: TimelineItem[] = []
  flow.forEach((item, index) => {
    if (textContent(item) !== undefined) {
      ;(showsText(index) ? resultItems : workItems).push(item)
    } else if (isCollapsibleWork(item, pendingPermissionIds, latestTaskSurfaceIndex)) {
      workItems.push(item)
    } else {
      resultItems.push(item)
    }
  })
  return { workItems, resultItems }
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
  const permissionResolutions = new Map<string, ACPPermission>()
  const latestPermissionRequest = new Map<string, number>()
  const latestTaskSurfaceEvent = new Map<string, number>()
  const latestToolEvent = new Map<string, number>()
  events.forEach((event, index) => {
    if (event.type === 'permission_request' && event.permission) {
      latestPermissionRequest.set(event.permission.id, index)
    }
    if (event.type === 'permission_response' && event.permission) {
      permissionResolutions.set(event.permission.id, event.permission)
    }
    const acp = event.acp
    if (acp && taskSurfaceFromEvent(event)) {
      latestTaskSurfaceEvent.set(acp.id, index)
    }
    if (event.type === 'acp' && acp?.tool_calls?.length) {
      latestToolEvent.set(acp.id, index)
    }
  })
  const renderedEvents = events
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
      const taskSurface = taskSurfaceFromEvent(event)
      if (event.type === 'artifact') return Boolean(event.artifact)
      if (event.type === 'loop_created') return Boolean(event.loop_created)
      if (!acp) {
        if (taskSurface) return true
        return Boolean(event.content || event.permission)
      }
      if (!hasVisibleACPSurface(event)) return false
      // This page's own running state has nothing to render — drop the event
      // instead of leaving an empty row.
      if (isWorkingStatusOnly(event) && acp.id === sessionId) return false
      if (taskSurface) {
        const isLatestTaskSurface = latestTaskSurfaceEvent.get(acp.id) === index
        if (!isLatestTaskSurface && !event.content && !acp.tool_calls?.length) return false
      }
      if (event.type === 'acp' && acp.tool_calls?.length && latestToolEvent.get(acp.id) !== index) {
        return Boolean(event.content || taskSurface)
      }
      return true
    })

  const pendingPermissionIds = new Set<string>()
  for (const event of events) {
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
  const isWorkingStatusItem = (item: TimelineItem) =>
    item.kind === 'event' && isWorkingStatusOnly(item.event)
  const pendingCards = merged.filter(isPendingCard)
  const workingStatusItems = merged.filter(isWorkingStatusItem)
  const chronological = merged.filter((item) => !isPendingCard(item) && !isWorkingStatusItem(item))
  // A live "working" indicator next to a question awaiting the user is noise.
  const anchored = [...(pendingCards.length ? [] : workingStatusItems), ...pendingCards]
  markEventHeaders([...chronological, ...anchored], sessionId)
  return {
    chronological,
    anchored,
    turns: groupTurns ? splitTurns(chronological) : [],
    permissionResolutions,
    latestTaskSurfaceEvent,
    pendingPermissionIds,
  }
}

// Coalesced events keep their latest copy whose seq changes per update; key by
// identity so streamed deltas patch in place instead of remounting.
export function stableEventKey(event: SessionEvent, eventIndex = 0): string {
  const coalesceKey = sessionEventCoalesceKey(event)
  if (coalesceKey) return coalesceKey
  if ((event.type === 'acp_message' || event.type === 'acp_thought') && event.acp?.id) {
    return `${event.type}:${event.acp.id}:${event.session_id}:${eventIndex}`
  }
  return `${event.session_id}:${event.seq ?? 'live'}`
}
