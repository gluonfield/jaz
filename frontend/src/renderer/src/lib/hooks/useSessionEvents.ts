import { useQueryClient } from '@tanstack/react-query'
import { useEffect, type RefObject } from 'react'
import { openSessionEvents } from '@/lib/api/sse'
import type { SessionEvent } from '@/lib/api/types'
import { keys } from '@/lib/query/keys'
import { mergeSessionEvent } from '@/lib/sessionEvents'

// Subscribes to a session's SSE stream while mounted; events accumulate in
// the query cache. streamingRef suppresses mid-turn message refetches on the
// page that is itself streaming — its live exchange already renders the turn.
// Cache writes are batched and sidebar invalidations debounced so a busy turn
// costs one render per flush, not one per event.
export function useSessionEvents(
  sessionId: string,
  streamingRef?: RefObject<boolean>,
  onEvent?: (event: SessionEvent) => void,
): void {
  const queryClient = useQueryClient()

  useEffect(() => {
    const refetchMessages = () => {
      if (streamingRef?.current) return
      queryClient.invalidateQueries({ queryKey: keys.sessionMessages(sessionId) })
      // Turn boundaries are when the working tree changes — refresh repo
      // state and the changes summary here instead of polling for them.
      queryClient.invalidateQueries({ queryKey: keys.sessionRepo(sessionId) })
    }
    let pending: SessionEvent[] = []
    let flushTimer: ReturnType<typeof setTimeout> | null = null
    let listsTimer: ReturnType<typeof setTimeout> | null = null
    const flush = () => {
      flushTimer = null
      const batch = pending
      pending = []
      queryClient.setQueryData<SessionEvent[]>(keys.sessionEvents(sessionId), (prev = []) =>
        batch.reduce(mergeSessionEvent, prev),
      )
    }
    const invalidateLists = () => {
      listsTimer = null
      queryClient.invalidateQueries({ queryKey: keys.sidebarSessions })
      queryClient.invalidateQueries({ queryKey: keys.allSessions })
    }
    const stop = openSessionEvents(sessionId, (event: SessionEvent) => {
      onEvent?.(event)
      // 'assistant' events are refresh signals, not transcript items.
      if (event.type === 'assistant') {
        refetchMessages()
      } else {
        pending.push(event)
        flushTimer ??= setTimeout(flush, 50)
        // turn finished: new rows were persisted
        const state = (event.acp?.state ?? '').toLowerCase()
        if (event.type === 'acp' && ['idle', 'failed', 'cancelled'].includes(state)) {
          refetchMessages()
        }
      }
      listsTimer ??= setTimeout(invalidateLists, 500)
    })
    return () => {
      stop()
      if (flushTimer !== null) clearTimeout(flushTimer)
      // Don't drop a partial batch on unmount: the cache outlives this page.
      if (pending.length) flush()
      if (listsTimer !== null) clearTimeout(listsTimer)
    }
  }, [sessionId, queryClient, streamingRef, onEvent])
}
