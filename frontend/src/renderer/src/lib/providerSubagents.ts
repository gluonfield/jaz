import type { ACPToolCall, ProviderSubagentEvent, SessionEvent } from '@/lib/api/types'

export interface ProviderSubagentView extends ProviderSubagentEvent {
  key: string
  updated_at: string
}

export function providerSubagentsFromEvents(events: SessionEvent[]): ProviderSubagentView[] {
  const byKey = new Map<string, ProviderSubagentView>()
  for (const event of events) {
    if (event.type !== 'provider_subagent' || !event.provider_subagent?.id) continue
    const subagent = event.provider_subagent
    const key = `${subagent.provider ?? ''}:${subagent.id}`
    const prev = byKey.get(key)
    byKey.set(key, {
      ...(prev ?? {}),
      ...mergeProviderSubagentEvent(prev, subagent),
      key,
      updated_at: event.at,
    })
  }
  return [...byKey.values()].sort((a, b) => {
    const active = subagentActiveRank(a) - subagentActiveRank(b)
    if (active) return active
    return eventTime(b.updated_at) - eventTime(a.updated_at)
  })
}

function providerSubagentKey(event: SessionEvent): string {
  const subagent = event.provider_subagent
  if (!subagent?.id) return ''
  return `${subagent.provider ?? ''}:${subagent.id}`
}

function looksLikeOpaqueToolID(text: string): boolean {
  return /^(?:toolu_|call_|tool[-_])[A-Za-z0-9_-]+$/.test(text)
}

function isUsefulToolSummary(summary: string): boolean {
  if (!summary || looksLikeOpaqueToolID(summary)) return false
  return summary !== 'Subagent message' && summary !== 'Subagent thinking'
}

function callNeedsTitle(call: ACPToolCall): boolean {
  return !call.title && !call.raw_input
}

export function applyProviderToolTitleFallbacks(events: SessionEvent[]): SessionEvent[] {
  const latestSummaryBySubagent = new Map<string, string>()
  const titleByToolID = new Map<string, string>()

  for (const event of events) {
    if (event.type !== 'provider_subagent') continue
    const summary = event.provider_subagent?.summary?.trim() ?? ''
    const subagentKey = providerSubagentKey(event)
    if (!summary || !subagentKey) continue
    if (looksLikeOpaqueToolID(summary)) {
      const title = latestSummaryBySubagent.get(subagentKey)
      if (title) titleByToolID.set(summary, title)
    } else if (isUsefulToolSummary(summary)) {
      latestSummaryBySubagent.set(subagentKey, summary)
    }
  }

  if (!titleByToolID.size) return events
  return events.map((event) => {
    const calls = event.acp?.tool_calls
    if (!calls?.length) return event
    let changed = false
    const nextCalls = calls.map((call) => {
      const title = call.id ? titleByToolID.get(call.id) : undefined
      if (!title || !callNeedsTitle(call)) return call
      changed = true
      return { ...call, title }
    })
    if (!changed) return event
    return { ...event, acp: event.acp ? { ...event.acp, tool_calls: nextCalls } : event.acp }
  })
}

export function mergeProviderSubagentEvent(
  prev: ProviderSubagentEvent | undefined,
  next: ProviderSubagentEvent,
): ProviderSubagentEvent {
  return {
    ...next,
    provider: next.provider || prev?.provider,
    thread_id: next.thread_id || prev?.thread_id,
    parent_id: next.parent_id || prev?.parent_id,
    name: next.name || prev?.name,
    role: next.role || prev?.role,
    status: next.status || prev?.status,
    summary: next.summary || prev?.summary,
    prompt: next.prompt || prev?.prompt,
    model: next.model || prev?.model,
    reasoning_effort: next.reasoning_effort || prev?.reasoning_effort,
    started_at_ms: next.started_at_ms ?? prev?.started_at_ms,
    completed_at_ms: next.completed_at_ms ?? prev?.completed_at_ms,
  }
}

function subagentActiveRank(subagent: ProviderSubagentView): number {
  const status = subagent.status?.toLowerCase()
  if (!status || status === 'running' || status === 'starting' || status === 'pending_init') return 0
  return 1
}

function eventTime(value: string): number {
  const time = Date.parse(value)
  return Number.isNaN(time) ? 0 : time
}
