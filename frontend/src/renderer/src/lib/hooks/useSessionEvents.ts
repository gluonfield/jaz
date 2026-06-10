import { useQueryClient } from '@tanstack/react-query'
import { useEffect, type RefObject } from 'react'
import { openSessionEvents } from '@/lib/api/sse'
import type { SessionEvent } from '@/lib/api/types'
import { keys } from '@/lib/query/keys'
import { mergeSessionEvent } from '@/lib/sessionEvents'

// Subscribes to a session's SSE stream while mounted; events accumulate in
// the query cache. streamingRef suppresses mid-turn message refetches on the
// page that is itself streaming — its live exchange already renders the turn.
export function useSessionEvents(sessionId: string, streamingRef?: RefObject<boolean>): void {
  const queryClient = useQueryClient()

  useEffect(() => {
    const refetchMessages = () => {
      if (streamingRef?.current) return
      queryClient.invalidateQueries({ queryKey: keys.sessionMessages(sessionId) })
    }
    const stop = openSessionEvents(sessionId, (event: SessionEvent) => {
      // 'assistant' events are refresh signals, not transcript items.
      if (event.type === 'assistant') {
        refetchMessages()
      } else {
        queryClient.setQueryData<SessionEvent[]>(keys.sessionEvents(sessionId), (prev = []) =>
          mergeSessionEvent(prev, event),
        )
        // turn finished: new rows were persisted
        const state = (event.acp?.state ?? '').toLowerCase()
        if (event.type === 'acp' && ['idle', 'failed', 'cancelled'].includes(state)) {
          refetchMessages()
        }
      }
      queryClient.invalidateQueries({ queryKey: keys.sidebarSessions })
      queryClient.invalidateQueries({ queryKey: keys.allSessions })
    })
    return stop
  }, [sessionId, queryClient, streamingRef])
}
