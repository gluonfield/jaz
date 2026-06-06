import { useQueryClient } from '@tanstack/react-query'
import { useEffect } from 'react'
import { openSessionEvents } from '../api/sse'
import type { SessionEvent } from '../api/types'
import { keys } from '../query/keys'

// Subscribes to a session's SSE stream for as long as the component is
// mounted. Events accumulate in the query cache (so remounts within the
// session keep history) and bump the sidebar list's freshness.
export function useSessionEvents(sessionId: string): void {
  const queryClient = useQueryClient()

  useEffect(() => {
    const stop = openSessionEvents(sessionId, (event: SessionEvent) => {
      queryClient.setQueryData<SessionEvent[]>(keys.sessionEvents(sessionId), (prev = []) => [
        ...prev,
        event,
      ])
      queryClient.invalidateQueries({ queryKey: keys.sidebarSessions })
      queryClient.invalidateQueries({ queryKey: keys.allSessions })
    })
    return stop
  }, [sessionId, queryClient])
}
