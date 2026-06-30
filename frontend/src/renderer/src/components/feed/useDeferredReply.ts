import { useQueryClient } from '@tanstack/react-query'
import { useCallback, useEffect, useRef, useState } from 'react'
import { useToast } from '@/components/ui/toast'
import { markThreadSeen } from '@/lib/api/feed'
import { mutateSessionQueue, uploadSessionAttachment } from '@/lib/api/sessions'
import { keys } from '@/lib/query/keys'
import { preparedSendMessage, type SendMessageOptions } from '@/lib/sendMessage'

export const COUNTDOWN_SECONDS = 3

export function useDeferredReply(threadId: string, onCommit: () => void) {
  const queryClient = useQueryClient()
  const toast = useToast()
  const [counting, setCounting] = useState(false)
  const countdownRef = useRef<{ resolve: () => void; reject: () => void; timer: ReturnType<typeof setTimeout> } | null>(null)

  const settle = useCallback((commit: boolean) => {
    const pending = countdownRef.current
    if (!pending) return
    clearTimeout(pending.timer)
    countdownRef.current = null
    if (commit) {
      pending.resolve()
    } else {
      setCounting(false)
      pending.reject(new Error('cancelled'))
    }
  }, [])

  useEffect(
    () => () => {
      const pending = countdownRef.current
      if (!pending) return
      clearTimeout(pending.timer)
      pending.reject(new Error('cancelled'))
    },
    [],
  )

  const send = useCallback(
    async (text: string, options: SendMessageOptions) => {
      const uploaded = options.files?.length
        ? await Promise.all(options.files.map((file) => uploadSessionAttachment(threadId, file)))
        : []
      const prepared = preparedSendMessage(options, uploaded)
      await markThreadSeen(threadId)
      await mutateSessionQueue(threadId, {
        op: 'append',
        message: {
          text,
          contexts: prepared.contexts,
          attachment_ids: prepared.attachmentIds,
          plan_requested: options.planRequested,
          goal_requested: options.goalRequested,
        },
      })
    },
    [threadId],
  )

  const commit = useCallback(
    (text: string, options: SendMessageOptions) => {
      onCommit()
      send(text, options).catch((error: Error) => {
        toast(`Couldn't send reply: ${error.message}`, 'danger')
        queryClient.invalidateQueries({ queryKey: keys.feed })
      })
    },
    [onCommit, send, toast, queryClient],
  )

  const sendDeferred = useCallback(
    async (text: string, options: SendMessageOptions = {}) => {
      if (!text.trim()) return
      if (countdownRef.current) {
        settle(true)
        return
      }
      await new Promise<void>((resolve, reject) => {
        const timer = setTimeout(() => settle(true), COUNTDOWN_SECONDS * 1000)
        countdownRef.current = { resolve, reject, timer }
        setCounting(true)
      })
      commit(text, options)
    },
    [commit, settle],
  )

  const sendNow = useCallback(
    (text: string, options: SendMessageOptions = {}) => {
      if (text.trim()) commit(text, options)
    },
    [commit],
  )

  const commitNow = useCallback(() => settle(true), [settle])
  const cancel = useCallback(() => settle(false), [settle])

  return { counting, sendDeferred, sendNow, commitNow, cancel }
}
