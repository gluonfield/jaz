import { queryOptions } from '@tanstack/react-query'
import { keys } from '../query/keys'
import { get, post } from './client'
import type { FeedItem, Session } from './types'

export const feedQuery = queryOptions({
  queryKey: keys.feed,
  queryFn: async () => {
    const data = await get<{ items: FeedItem[] | null }>('/v1/feed')
    return data.items ?? []
  },
  refetchInterval: (query) =>
    query.state.data?.some((item) => item.status === 'running') ? 5_000 : 15_000,
})

export function markThreadSeen(id: string): Promise<Session> {
  return post<Session>(`/v1/sessions/${id}/seen`)
}
