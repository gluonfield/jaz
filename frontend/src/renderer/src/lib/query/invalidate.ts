import type { QueryClient } from '@tanstack/react-query'
import { keys } from './keys'

export function invalidateSessionLists(
  queryClient: QueryClient,
  opts: { archived?: boolean; session?: string } = {},
) {
  queryClient.invalidateQueries({ queryKey: keys.sidebarSessions })
  // Bots are threads too: pins, names, and unread state ride the same writes.
  queryClient.invalidateQueries({ queryKey: keys.bots })
  // Archiving, replying, and renaming all change which threads are unread.
  queryClient.invalidateQueries({ queryKey: keys.feed })
  if (opts.archived) queryClient.invalidateQueries({ queryKey: keys.archivedSessions })
  if (opts.session) {
    queryClient.invalidateQueries({ queryKey: keys.session(opts.session), exact: true })
  }
}

export function invalidateSessionTurnQueries(
  queryClient: QueryClient,
  sessionId: string,
  isCloud: boolean,
  opts: { includeOverview?: boolean; includeLoops?: boolean } = {},
) {
  queryClient.invalidateQueries({ queryKey: keys.usage })
  if (isCloud) {
    queryClient.invalidateQueries({ queryKey: keys.cloudUsageSummary })
    queryClient.invalidateQueries({ queryKey: keys.cloudThreadRepo(sessionId) })
    return
  }
  queryClient.invalidateQueries({ queryKey: keys.sessionRepo(sessionId) })
  if (opts.includeOverview) queryClient.invalidateQueries({ queryKey: keys.sessionOverview(sessionId) })
  if (opts.includeLoops) queryClient.invalidateQueries({ queryKey: keys.loops })
}
