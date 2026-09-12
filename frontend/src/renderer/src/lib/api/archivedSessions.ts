import { infiniteQueryOptions } from '@tanstack/react-query'
import { get } from '@/lib/api/client'
import type { Session } from '@/lib/api/types'
import { keys } from '@/lib/query/keys'

export const archivedSessionsQuery = (query: string) => infiniteQueryOptions({
  queryKey: [...keys.archivedSessions, query],
  initialPageParam: '',
  queryFn: ({ pageParam, signal }) => {
    const params = new URLSearchParams({
      archived: 'true',
      include_children: 'true',
      limit: '40',
      q: query,
    })
    if (pageParam) params.set('cursor', pageParam)
    return get<{ sessions: Session[]; next_cursor?: string }>(`/v1/sessions?${params}`, { signal })
  },
  getNextPageParam: (page) => page.next_cursor || undefined,
})
