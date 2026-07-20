import type { ACPEvent, ACPJobSnapshot, SessionEvent } from '@/lib/api/types'
import { isParentChildACPEvent } from '@/lib/sessionEvents'

type SpawnedThreadSource = Pick<
  ACPJobSnapshot,
  'id' | 'slug' | 'title' | 'acp_agent' | 'state' | 'model' | 'reasoning_effort' | 'last_event_at' | 'updated_at'
> & { archived?: boolean }

export interface SpawnedThreadView {
  key: string
  id: string
  slug: string
  title?: string
  agent: string
  state: string
  model?: string
  reasoning_effort?: string
  archived?: boolean
  updated_at: string
}

export function spawnedThreadsFromSources(
  children: SpawnedThreadSource[] | undefined,
  events: SessionEvent[],
): SpawnedThreadView[] {
  const byID = new Map<string, SpawnedThreadView>()
  for (const child of children ?? []) {
    if (!child.id) continue
    byID.set(child.id, threadViewFromSnapshot(child))
  }
  for (const event of events) {
    if (event.type !== 'acp' || !event.acp?.id || !isParentChildACPEvent(event)) continue
    const thread = threadViewFromACP(event.acp, event.at)
    const prev = byID.get(thread.id)
    byID.set(thread.id, prev ? mergeSpawnedThread(prev, thread) : thread)
  }
  return [...byID.values()].sort((a, b) => {
    const active = threadActiveRank(a) - threadActiveRank(b)
    if (active) return active
    return eventTime(b.updated_at) - eventTime(a.updated_at)
  })
}

function threadViewFromSnapshot(child: SpawnedThreadSource): SpawnedThreadView {
  return {
    key: child.id,
    id: child.id,
    slug: child.slug,
    title: child.title,
    agent: child.acp_agent,
    state: child.state,
    model: child.model,
    reasoning_effort: child.reasoning_effort,
    archived: child.archived,
    updated_at: child.last_event_at || child.updated_at,
  }
}

function threadViewFromACP(acp: ACPEvent, updatedAt: string): SpawnedThreadView {
  return {
    key: acp.id,
    id: acp.id,
    slug: acp.slug,
    title: acp.title,
    agent: acp.agent,
    state: acp.state,
    model: acp.model,
    reasoning_effort: acp.reasoning_effort,
    updated_at: updatedAt,
  }
}

function mergeSpawnedThread(prev: SpawnedThreadView, next: SpawnedThreadView): SpawnedThreadView {
  const nextIsNewer = eventTime(next.updated_at) >= eventTime(prev.updated_at)
  const base = nextIsNewer ? next : prev
  const fallback = nextIsNewer ? prev : next
  return {
    ...base,
    slug: base.slug || fallback.slug,
    title: base.title || fallback.title,
    agent: base.agent || fallback.agent,
    state: base.state || fallback.state,
    model: base.model || fallback.model,
    reasoning_effort: base.reasoning_effort || fallback.reasoning_effort,
    archived: base.archived || fallback.archived,
    updated_at: laterTime(prev.updated_at, next.updated_at),
  }
}

function threadActiveRank(thread: SpawnedThreadView): number {
  const state = thread.state.toLowerCase()
  return !state || state === 'running' || state === 'starting' ? 0 : 1
}

function eventTime(value: string): number {
  const time = Date.parse(value)
  return Number.isNaN(time) ? 0 : time
}

function laterTime(a: string, b: string): string {
  return eventTime(b) > eventTime(a) ? b : a
}
