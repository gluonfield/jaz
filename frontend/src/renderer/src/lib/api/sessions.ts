import { queryOptions } from '@tanstack/react-query'
import { keys } from '../query/keys'
import { get, post } from './client'
import type { Session, SessionMessages } from './types'

export function createSession(): Promise<Session> {
  return post<Session>('/v1/sessions', {})
}

export const SIDEBAR_SESSION_LIMIT = 7

function sessionTime(session: Session): number {
  const ms = Date.parse(session.updated_at)
  return Number.isNaN(ms) ? 0 : ms
}

function compareSessions(a: Session, b: Session): number {
  return sessionTime(b) - sessionTime(a) || a.id.localeCompare(b.id)
}

export function groupSessionsForDisplay(sessions: Session[]): Session[] {
  const byID = new Map(sessions.map((session) => [session.id, session]))
  const children = new Map<string, Session[]>()
  const roots: Session[] = []

  for (const session of sessions) {
    if (session.parent_id && session.parent_id !== session.id && byID.has(session.parent_id)) {
      children.set(session.parent_id, [...(children.get(session.parent_id) ?? []), session])
    } else {
      roots.push(session)
    }
  }

  const groupTimes = new Map<string, number>()
  const groupTime = (session: Session, visiting = new Set<string>()): number => {
    const cached = groupTimes.get(session.id)
    if (cached !== undefined) return cached
    if (visiting.has(session.id)) return sessionTime(session)

    visiting.add(session.id)
    let latest = sessionTime(session)
    for (const child of children.get(session.id) ?? []) {
      latest = Math.max(latest, groupTime(child, visiting))
    }
    visiting.delete(session.id)
    groupTimes.set(session.id, latest)
    return latest
  }

  const compareGroups = (a: Session, b: Session): number =>
    groupTime(b) - groupTime(a) || compareSessions(a, b)

  const ordered: Session[] = []
  const emitted = new Set<string>()
  const append = (session: Session) => {
    if (emitted.has(session.id)) return
    emitted.add(session.id)
    ordered.push(session)
    for (const child of [...(children.get(session.id) ?? [])].sort(compareGroups)) append(child)
  }

  for (const root of [...roots].sort(compareGroups)) append(root)
  for (const session of [...sessions].sort(compareGroups)) append(session)
  return ordered
}

export const sidebarSessionsQuery = queryOptions({
  queryKey: keys.sidebarSessions,
  queryFn: async () => {
    const data = await get<{ sessions: Session[] | null }>('/v1/sessions?include_children=true')
    return groupSessionsForDisplay(data.sessions ?? [])
  },
  // Tighten the poll while a thread is running so status dots stay live.
  refetchInterval: (query) =>
    query.state.data?.some((s) => s.status === 'running') ? 3_000 : 15_000,
})

export const allSessionsQuery = queryOptions({
  queryKey: keys.allSessions,
  queryFn: async () => {
    const data = await get<{ sessions: Session[] | null }>('/v1/sessions?include_children=true')
    return groupSessionsForDisplay(data.sessions ?? [])
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
