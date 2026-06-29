import type { QueryClient } from '@tanstack/react-query'
import { keys } from './keys'

// The session list views are separate caches, so refreshing them after a
// mutation means invalidating each — a single `['sessions']` prefix would also
// drop per-session details, messages, and repo state we want to keep.
export function invalidateSessionLists(
  queryClient: QueryClient,
  opts: { archived?: boolean; session?: string } = {},
) {
  queryClient.invalidateQueries({ queryKey: keys.sidebarSessions })
  queryClient.invalidateQueries({ queryKey: keys.allSessions })
  // Archiving, replying, and renaming all change which threads are unread.
  queryClient.invalidateQueries({ queryKey: keys.feed })
  if (opts.archived) queryClient.invalidateQueries({ queryKey: keys.archivedSessions })
  if (opts.session) {
    queryClient.invalidateQueries({ queryKey: keys.session(opts.session), exact: true })
  }
}
