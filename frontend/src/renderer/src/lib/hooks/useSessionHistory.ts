import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useCallback, useEffect, useRef, useState } from 'react'
import { getSessionMessagesPage } from '@/lib/api/sessions'
import { ApiError } from '@/lib/api/client'
import type { SessionMessages } from '@/lib/api/types'
import { keys } from '@/lib/query/keys'
import { mergeEarlierHistory, mergeLatestHistory } from '@/lib/sessionHistory'

export function useSessionHistory(sessionId: string, onLoadError: (message: string) => void) {
  const queryClient = useQueryClient()
  const request = useRef<AbortController | null>(null)
  const [loadingEarlierHistory, setLoadingEarlierHistory] = useState(false)
  const query = useQuery<SessionMessages>({
    queryKey: keys.sessionMessages(sessionId),
    queryFn: async ({ signal }) => {
      const latest = await getSessionMessagesPage(sessionId, {}, signal)
      return mergeLatestHistory(
        queryClient.getQueryData<SessionMessages>(keys.sessionMessages(sessionId)),
        latest,
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
    const current = queryClient.getQueryData<SessionMessages>(keys.sessionMessages(sessionId))
    if (!current || (!current.before_message_seq && !current.before_event_seq) || request.current) return false
    const controller = new AbortController()
    request.current = controller
    setLoadingEarlierHistory(true)
    try {
      const page = await getSessionMessagesPage(sessionId, {
        beforeMessageSeq: current.before_message_seq,
        beforeEventSeq: current.before_event_seq,
        historyRevision: current.history_revision,
        turns: 24,
      }, controller.signal)
      queryClient.setQueryData<SessionMessages>(keys.sessionMessages(sessionId), (cached) =>
        cached?.history_revision === page.history_revision ? mergeEarlierHistory(cached, page) : cached,
      )
      return page.messages.length > 0 || page.events.length > 0
    } catch (error) {
      if (controller.signal.aborted) return false
      if (error instanceof ApiError && error.status === 409) {
        await refetch()
        return false
      }
      onLoadError((error as Error).message)
      return false
    } finally {
      if (request.current === controller) request.current = null
      setLoadingEarlierHistory(false)
    }
  }, [onLoadError, queryClient, refetch, sessionId])

  return { ...query, loadingEarlierHistory, loadEarlierHistory }
}
