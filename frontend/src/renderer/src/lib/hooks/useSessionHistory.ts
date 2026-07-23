import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useCallback, useEffect, useRef, useState } from 'react'
import { getSessionMessagesPage } from '@/lib/api/sessions'
import { ApiError } from '@/lib/api/client'
import type { SessionMessages } from '@/lib/api/types'
import { keys } from '@/lib/query/keys'
import { loadCompleteHistoryBatch, mergeEarlierHistory, mergeLatestHistory } from '@/lib/sessionHistory'

function fetchCompleteHistoryBatch(
  sessionId: string,
  current: SessionMessages,
  signal: AbortSignal,
): Promise<SessionMessages> {
  return loadCompleteHistoryBatch(current, (cursor) =>
    getSessionMessagesPage(sessionId, {
      beforeMessageSeq: cursor.before_message_seq,
      beforeEventSeq: cursor.before_event_seq,
      historyRevision: cursor.history_revision,
      turns: 24,
    }, signal),
  )
}

export function useSessionHistory(sessionId: string, onLoadError: (message: string) => void) {
  const queryClient = useQueryClient()
  const request = useRef<AbortController | null>(null)
  const [loadingEarlierHistory, setLoadingEarlierHistory] = useState(false)
  const query = useQuery<SessionMessages>({
    queryKey: keys.sessionMessages(sessionId),
    queryFn: async ({ signal }) => {
      const latest = await getSessionMessagesPage(sessionId, {}, signal)
      const complete = await fetchCompleteHistoryBatch(sessionId, latest, signal)
      return mergeLatestHistory(
        queryClient.getQueryData<SessionMessages>(keys.sessionMessages(sessionId)),
        complete,
      )
    },
  })
  const { refetch } = query

  useEffect(() => {
    request.current?.abort()
    request.current = null
    setLoadingEarlierHistory(false)
    return () => {
      request.current?.abort()
      request.current = null
    }
  }, [sessionId])

  const loadEarlierHistory = useCallback(async (): Promise<boolean> => {
    let current = queryClient.getQueryData<SessionMessages>(keys.sessionMessages(sessionId))
    if (!current?.has_earlier || request.current) return false
    const controller = new AbortController()
    request.current = controller
    setLoadingEarlierHistory(true)
    try {
      for (let attempt = 0; attempt < 2; attempt += 1) {
        try {
          const batch = await fetchCompleteHistoryBatch(sessionId, current, controller.signal)
          const cached = queryClient.getQueryData<SessionMessages>(keys.sessionMessages(sessionId))
          if (cached?.history_revision !== batch.history_revision) {
            if (cached?.has_earlier && attempt === 0) {
              current = cached
              continue
            }
            if (cached?.has_earlier) onLoadError('Session history changed again while loading')
            return false
          }
          queryClient.setQueryData(keys.sessionMessages(sessionId), mergeEarlierHistory(cached, batch))
          return true
        } catch (error) {
          if (controller.signal.aborted) return false
          if (error instanceof ApiError && error.status === 409 && attempt === 0) {
            const refreshed = await refetch()
            if (refreshed.error) {
              onLoadError(refreshed.error.message)
              return false
            }
            if (!refreshed.data?.has_earlier) return false
            current = refreshed.data
            continue
          }
          onLoadError((error as Error).message)
          return false
        }
      }
      return false
    } finally {
      if (request.current === controller) request.current = null
      setLoadingEarlierHistory(false)
    }
  }, [onLoadError, queryClient, refetch, sessionId])

  return { ...query, loadingEarlierHistory, loadEarlierHistory }
}
