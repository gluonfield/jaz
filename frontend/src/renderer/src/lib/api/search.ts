import { apiFetch } from './client'
import { readAPIResponse } from '@/lib/api/response'
import type { ConnectionSearchSection, ThreadSearchResult } from './types'

export async function searchThreads(input: {
  query: string
  includeArchived?: boolean
  limit?: number
  signal?: AbortSignal
}): Promise<ThreadSearchResult[]> {
  const params = new URLSearchParams({ q: input.query })
  if (input.includeArchived) params.set('include_archived', 'true')
  if (input.limit) params.set('limit', String(input.limit))
  const res = await apiFetch(`/v1/search/threads?${params}`, { signal: input.signal })
  const data = await readAPIResponse<{ results?: ThreadSearchResult[] | null }>(res)
  return data.results ?? []
}

export async function searchConnections(query: string, signal?: AbortSignal): Promise<ConnectionSearchSection[]> {
  const res = await apiFetch(`/v1/search/connections?${new URLSearchParams({ q: query })}`, { signal })
  return (await readAPIResponse<{ sections: ConnectionSearchSection[] }>(res)).sections
}
