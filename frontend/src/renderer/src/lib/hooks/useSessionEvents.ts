import { useQueryClient } from '@tanstack/react-query'
import { useEffect } from 'react'
import { isCloudApiSession } from '@/lib/api/client'
import { openSessionEvents } from '@/lib/api/sse'
import type { SessionEvent, SessionMessages } from '@/lib/api/types'
import { invalidateSessionTurnQueries } from '@/lib/query/invalidate'
import { keys } from '@/lib/query/keys'
import { isSessionRefreshEventType, mergeSessionEvent } from '@/lib/sessionEvents'

// Subscribes to a session's SSE stream while mounted; events accumulate in
// the query cache. Cache writes are batched and sidebar invalidations
// debounced so a busy turn costs one render per flush, not one per event.
export function useSessionEvents(
  sessionId: string,
  latestEventSeq: number | undefined,
  onEvent?: (event: SessionEvent) => void,
): void {
  const queryClient = useQueryClient()
  const afterSeq = latestEventSeq

  useEffect(() => {
    if (!afterSeq) return
    queryClient.setQueryData<SessionEvent[]>(keys.sessionEvents(sessionId), (prev = []) => {
      const next = prev.filter((event) => !event.seq || event.seq > afterSeq)
      return next.length === prev.length ? prev : next
    })
  }, [sessionId, afterSeq, queryClient])

  useEffect(() => {
    if (afterSeq === undefined) return
    const refetchMessages = () => {
      queryClient.invalidateQueries({ queryKey: keys.sessionMessages(sessionId) })
      const isCloud =
        isCloudApiSession() ||
        queryClient.getQueryData<SessionMessages>(keys.sessionMessages(sessionId))?.session.backend === 'cloud'
      invalidateSessionTurnQueries(queryClient, sessionId, isCloud, { includeOverview: true, includeLoops: true })
    }
    let pending: SessionEvent[] = []
    let flushTimer: ReturnType<typeof setTimeout> | null = null
    let listsTimer: ReturnType<typeof setTimeout> | null = null
    let overviewChanged = false
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
      if (overviewChanged) {
        overviewChanged = false
        queryClient.invalidateQueries({ queryKey: keys.sessionOverview(sessionId) })
      }
    }
    const stop = openSessionEvents(sessionId, afterSeq, (event: SessionEvent) => {
      onEvent?.(event)
      overviewChanged ||= ['agent_session', 'agent_task', 'provider_subagent'].includes(event.type) || event.artifact?.kind === 'plugin-view'
      // 'assistant' and user/session lifecycle events are refresh signals, not transcript rows.
      if (event.type === 'assistant') {
        refetchMessages()
      } else if (isSessionRefreshEventType(event.type)) {
        queryClient.invalidateQueries({ queryKey: keys.sessionMessages(sessionId) })
      } else if (event.type === 'turn_queued' || event.type === 'turn.queued') {
        // Queued turn ack carries no transcript row until the turn starts.
      } else {
        pending.push(event)
        flushTimer ??= setTimeout(flush, 50)
        // turn finished: settle running state immediately and refetch persisted rows
        const state = (event.acp?.state ?? '').toLowerCase()
        if (event.type === 'acp' && ['idle', 'failed', 'cancelled'].includes(state)) {
          const nextStatus = state === 'failed' ? 'error' : state === 'cancelled' ? 'interrupted' : 'idle'
          queryClient.setQueryData<SessionMessages>(keys.sessionMessages(sessionId), (prev) =>
            prev
              ? {
                  ...prev,
                  acp_state: state,
                  session: prev.session.status === 'running' ? { ...prev.session, status: nextStatus } : prev.session,
                }
              : prev,
          )
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
  }, [sessionId, afterSeq, queryClient, onEvent])
}
