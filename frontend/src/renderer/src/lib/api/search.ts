import { apiFetch, isCloudApiSession } from './client'
import { cloudThreadToSession, listCloudThreads } from './cloudThreads'
import { readAPIResponse } from '@/lib/api/response'
import type { ConnectionSearchSection, ThreadSearchResult } from './types'

export async function searchThreads(input: {
  query: string
  includeArchived?: boolean
  limit?: number
  signal?: AbortSignal
}): Promise<ThreadSearchResult[]> {
  if (isCloudApiSession()) {
    const needle = input.query.trim().toLowerCase()
    const sessions = (await listCloudThreads(undefined, input.signal))
      .map(cloudThreadToSession)
      .filter((s) => (input.includeArchived || !s.archived))
      .filter((s) => !needle || (s.title ?? s.slug ?? s.id).toLowerCase().includes(needle))
      .slice(0, input.limit ?? 20)
    return sessions.map((session) => ({
      thread_id: session.id,
      thread_slug: session.slug,
      thread_title: session.title,
      thread_status:
        session.status === 'running' || session.status === 'error' ? session.status : 'idle',
      thread_runtime: 'acp',
      thread_agent: session.runtime_ref?.agent,
      archived: session.archived,
      snippet: session.title || session.slug || session.id,
      updated_at: session.updated_at,
      last_attention_at: session.last_attention_at,
    }))
  }
  const params = new URLSearchParams({ q: input.query })
  if (input.includeArchived) params.set('include_archived', 'true')
  if (input.limit) params.set('limit', String(input.limit))
  const res = await apiFetch(`/v1/search/threads?${params}`, { signal: input.signal })
  const data = await readAPIResponse<{ results?: ThreadSearchResult[] | null }>(res)
  return data.results ?? []
}

export async function searchConnections(query: string, signal?: AbortSignal): Promise<ConnectionSearchSection[]> {
  if (isCloudApiSession()) return []
  const res = await apiFetch(`/v1/search/connections?${new URLSearchParams({ q: query })}`, { signal })
  return (await readAPIResponse<{ sections: ConnectionSearchSection[] }>(res)).sections
}
