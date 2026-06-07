import { useQueryClient } from '@tanstack/react-query'
import { useEffect, type RefObject } from 'react'
import { openSessionEvents } from '../api/sse'
import type { SessionEvent } from '../api/types'
import { keys } from '../query/keys'

function eventCoalesceKey(event: SessionEvent): string {
  if (event.type === 'acp' && event.acp?.id) {
    if (event.acp.plan?.length) return `acp_plan:${event.acp.id}`
    if (event.acp.tool_calls?.length) return `acp_tools:${event.acp.id}`
    if (event.acp.error) return `acp_error:${event.acp.id}`
    return `acp_status:${event.acp.id}`
  }
  if (event.type === 'acp_tool' && event.acp?.id && event.acp.tool_calls?.[0]?.id) {
    return `acp_tool:${event.acp.id}:${event.acp.tool_calls[0].id}`
  }
  if ((event.type === 'permission_request' || event.type === 'permission_response') && event.permission?.id) {
    return `${event.type}:${event.permission.id}`
  }
  return ''
}

function mergeSessionEvent(prev: SessionEvent[], event: SessionEvent): SessionEvent[] {
  // The store-assigned seq identifies an event exactly (replays, reconnects).
  if (event.seq) {
    const seqIndex = prev.findIndex(
      (item) => item.seq === event.seq && item.session_id === event.session_id,
    )
    if (seqIndex !== -1) {
      const next = [...prev]
      next[seqIndex] = event
      return next
    }
  }
  const key = eventCoalesceKey(event)
  if (!key) return [...prev, event]
  const index = prev.findIndex((item) => eventCoalesceKey(item) === key)
  if (index === -1) return [...prev, event]
  const next = [...prev]
  next[index] = event
  return next
}

// Subscribes to a session's SSE stream for as long as the component is
// mounted. Events accumulate in the query cache (so remounts within the
// session keep history) and bump the sidebar list's freshness.
//
// streamingRef: while this page itself streams a turn, its live exchange is
// the sole renderer of the in-flight messages — refetching mid-turn would
// show the persisted user/assistant rows next to the live copies. Other
// windows (and pages opened after a refresh) keep refetching per round.
export function useSessionEvents(sessionId: string, streamingRef?: RefObject<boolean>): void {
  const queryClient = useQueryClient()

  useEffect(() => {
    const refetchMessages = () => {
      if (streamingRef?.current) return
      queryClient.invalidateQueries({ queryKey: keys.sessionMessages(sessionId) })
    }
    const stop = openSessionEvents(sessionId, (event: SessionEvent) => {
      // Coordinator follow-ups land in the message store; the event is just
      // the "refetch now" signal, not a transcript item of its own.
      if (event.type === 'assistant') {
        refetchMessages()
      } else {
        queryClient.setQueryData<SessionEvent[]>(keys.sessionEvents(sessionId), (prev = []) =>
          mergeSessionEvent(prev, event),
        )
        // A finished turn means new rows were persisted (user echoes,
        // completion summaries) and the ACP state went idle — refresh both.
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
