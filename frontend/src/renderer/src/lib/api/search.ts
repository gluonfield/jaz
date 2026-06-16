import { apiFetch, ApiError } from './client'
import type { ThreadSearchResult } from './types'

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
  if (!res.ok) {
    let message = `${res.status} ${res.statusText}`
    try {
      const body = (await res.json()) as { error?: string }
      if (body.error) message = body.error
    } catch {
      // keep status text
    }
    throw new ApiError(res.status, message)
  }
  const data = (await res.json()) as { results?: ThreadSearchResult[] | null }
  return data.results ?? []
}
