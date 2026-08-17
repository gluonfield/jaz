import { useQueryClient } from '@tanstack/react-query'
import { useCallback } from 'react'
import { useToast } from '@/components/ui/toast'
import { mutateSessionQueue, type QueueMutation } from '@/lib/api/sessions'
import type { QueuedAction } from '@/lib/api/types'
import { invalidateSessionLists } from '@/lib/query/invalidate'
import { keys } from '@/lib/query/keys'
import { queuedActionMessage } from '@/lib/sessionQueue'
import { appendSessionPrompt } from '@/lib/sessionPrompt'
import type { SendMessageHandler } from '@/lib/sendMessage'

type QueueAppendMutation = Extract<QueueMutation, { op: 'append' }>
type QueueAppendRunner = (mutation: QueueAppendMutation) => Promise<void>

export function useQueueAppender(sessionId: string, append?: QueueAppendRunner) {
  const queryClient = useQueryClient()
  const toast = useToast()

  const appendQueue = useCallback(async (mutation: QueueAppendMutation) => {
    if (append) {
      await append(mutation)
      return
    }
    await mutateSessionQueue(sessionId, mutation)
    queryClient.invalidateQueries({ queryKey: keys.sessionMessages(sessionId) })
    invalidateSessionLists(queryClient, { session: sessionId })
  }, [append, queryClient, sessionId])

  const queuePrompt: SendMessageHandler = useCallback((text, options = {}) => {
    return appendSessionPrompt(sessionId, text, options, 'queued', appendQueue)
      .then(() => undefined)
      .catch((error) => {
        toast(`Queue update failed: ${(error as Error).message}`, 'danger')
        throw error
      })
  }, [appendQueue, sessionId, toast])

  const queueAction = useCallback((action: QueuedAction, label: string) => {
    return appendQueue({ op: 'append', message: queuedActionMessage(action, label) })
      .catch((error) => {
        toast(`Queue update failed: ${(error as Error).message}`, 'danger')
        throw error
      })
  }, [appendQueue, toast])

  return { queuePrompt, queueAction }
}
