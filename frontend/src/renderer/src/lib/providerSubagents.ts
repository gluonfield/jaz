import type {
  ACPToolCall,
  ProviderSubagentEvent,
  SessionEvent,
  SessionOverviewSubagent,
} from '@/lib/api/types'

export type ProviderSubagentView = SessionOverviewSubagent

export function providerSubagentsFromSources(
  stored: SessionOverviewSubagent[] | undefined,
  events: SessionEvent[],
): ProviderSubagentView[] {
  const byKey = new Map<string, ProviderSubagentView>()
  for (const subagent of stored ?? []) byKey.set(subagent.key, subagent)
  for (const event of events) {
    if (event.type !== 'provider_subagent' || !event.provider_subagent?.id) continue
    const subagent = event.provider_subagent
    const key = providerSubagentKey(event)
    if (!key) continue
    const previous = byKey.get(key)
    const current: ProviderSubagentView = {
      ...subagent,
      key,
      seq: event.seq ?? 0,
      updated_at: event.at,
    }
    byKey.set(key, previous ? mergeProviderSubagentView(previous, current) : current)
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
  return event.projection_key || `provider_subagent:${subagent.provider ?? ''}:${subagent.id}`
}

export function looksLikeOpaqueToolID(text: string): boolean {
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

function mergeProviderSubagentEvent(
  prev: ProviderSubagentEvent | undefined,
  next: ProviderSubagentEvent,
): ProviderSubagentEvent {
  return {
    ...next,
    provider: next.provider || prev?.provider,
    thread_id: next.thread_id || prev?.thread_id,
    parent_id: next.parent_id || prev?.parent_id,
    name: next.name || prev?.name,
    task: next.task || prev?.task,
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

function mergeProviderSubagentView(
  previous: ProviderSubagentView,
  current: ProviderSubagentView,
): ProviderSubagentView {
  const currentIsNewer = subagentUpdateIsNewer(previous, current)
  const latest = currentIsNewer ? current : previous
  const earlier = currentIsNewer ? previous : current
  return {
    ...mergeProviderSubagentEvent(earlier, latest),
    key: latest.key,
    seq: Math.max(previous.seq, current.seq),
    updated_at: laterTime(previous.updated_at, current.updated_at),
  }
}

function subagentUpdateIsNewer(previous: ProviderSubagentView, current: ProviderSubagentView): boolean {
  if (previous.seq && current.seq) return current.seq > previous.seq
  return eventTime(current.updated_at) >= eventTime(previous.updated_at)
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

function laterTime(a: string, b: string): string {
  return eventTime(b) > eventTime(a) ? b : a
}
