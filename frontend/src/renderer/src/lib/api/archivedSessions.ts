import { infiniteQueryOptions } from '@tanstack/react-query'
import { get, isCloudApiSession } from '@/lib/api/client'
import { cloudThreadToSession, listCloudThreads } from '@/lib/api/cloudThreads'
import type { Session } from '@/lib/api/types'
import { keys } from '@/lib/query/keys'

export const archivedSessionsQuery = (query: string) => infiniteQueryOptions({
  queryKey: [...keys.archivedSessions, query],
  initialPageParam: '',
  queryFn: async ({ pageParam, signal }) => {
    if (isCloudApiSession()) {
      const needle = query.trim().toLowerCase()
      const sessions = (await listCloudThreads(undefined, signal))
        .map(cloudThreadToSession)
        .filter((s) => s.archived && (!needle || (s.title ?? s.slug ?? s.id).toLowerCase().includes(needle)))
      return { sessions }
    }
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
