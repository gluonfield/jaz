import { queryOptions } from '@tanstack/react-query'
import { keys } from '../query/keys'
import { get, post } from './client'
import type { Session, SessionMessages } from './types'

export function createSession(): Promise<Session> {
  return post<Session>('/v1/sessions', {})
}

export const SIDEBAR_SESSION_LIMIT = 7

export const rootSessionsQuery = queryOptions({
  queryKey: keys.rootSessions,
  queryFn: async () => {
    const data = await get<{ sessions: Session[] | null }>(
      `/v1/sessions?root=true&limit=${SIDEBAR_SESSION_LIMIT}`,
    )
    return data.sessions ?? []
  },
  refetchInterval: 15_000,
})

export const allSessionsQuery = queryOptions({
  queryKey: keys.allSessions,
  queryFn: async () => {
    const data = await get<{ sessions: Session[] | null }>('/v1/sessions?root=true')
    return data.sessions ?? []
  },
})

export const sessionMessagesQuery = (id: string) =>
  queryOptions({
    queryKey: keys.sessionMessages(id),
    queryFn: async () => {
      // Go marshals empty slices as null; normalize once here.
      const data = await get<SessionMessages>(`/v1/sessions/${id}/messages`)
      return {
        ...data,
        messages: data.messages ?? [],
        activity: data.activity ?? [],
      }
    },
  })

export const healthQuery = queryOptions({
  queryKey: keys.health,
  queryFn: () => get<{ ok: boolean }>('/health'),
  retry: false,
  refetchInterval: (query) => (query.state.status === 'error' ? 3_000 : 30_000),
})
