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
