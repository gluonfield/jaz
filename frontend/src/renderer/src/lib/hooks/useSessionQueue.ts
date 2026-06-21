import { useQueryClient } from '@tanstack/react-query'
import { useCallback, useEffect, useRef } from 'react'
import { useToast } from '@/components/ui/toast'
import { mutateSessionQueue, type QueueMutation, uploadSessionAttachment } from '@/lib/api/sessions'
import type { QueuedMessage, QueuedMessageInput, Session, SessionMessages } from '@/lib/api/types'
import { keys } from '@/lib/query/keys'
import type { SendMessageOptions } from '@/lib/sendMessage'

export function useSessionQueue({
  sessionId,
  session,
  acpState,
  streaming,
  onSend,
}: {
  sessionId: string
  session?: Session
  acpState?: string
  streaming: boolean
  onSend: (text: string, options?: SendMessageOptions) => void
}) {
  const queryClient = useQueryClient()
  const toast = useToast()
  const mutationChain = useRef<Promise<unknown>>(Promise.resolve())
  const queuedPrompts = normalizeQueuedPrompts(session?.queued_messages ?? [])
  const running = isSessionRunning({ session, acpState, streaming })

  useEffect(() => {
    mutationChain.current = Promise.resolve()
  }, [sessionId])

  const mutateQueue = useCallback((mutation: QueueMutation) => {
    const run = async () => {
      try {
        const updated = await mutateSessionQueue(sessionId, mutation)
        queryClient.setQueryData<SessionMessages>(keys.sessionMessages(sessionId), (prev) =>
          prev ? { ...prev, session: { ...prev.session, ...updated } } : prev,
        )
        queryClient.invalidateQueries({ queryKey: keys.sidebarSessions })
        queryClient.invalidateQueries({ queryKey: keys.allSessions })
        return updated.queued_messages ?? []
      } catch (error) {
        queryClient.invalidateQueries({ queryKey: keys.sessionMessages(sessionId) })
        throw error
      }
    }
    const next = mutationChain.current.then(run, run)
    mutationChain.current = next.catch(() => undefined)
    return next
  }, [queryClient, sessionId])

  const showQueueError = useCallback((error: unknown) => {
    toast(`Queue update failed: ${(error as Error).message}`, 'danger')
  }, [toast])

  const send = useCallback((text: string, options: SendMessageOptions = {}) => {
    if (running) {
      void (async () => {
        const uploaded = options.files?.length
          ? await Promise.all(options.files.map((file) => uploadSessionAttachment(sessionId, file)))
          : []
        const attachmentIDs = [
          ...(options.attachments ?? []).map((attachment) => attachment.id),
          ...uploaded.map((attachment) => attachment.id),
        ]
        const prompt = normalizeQueuedPrompt({
          text,
          quotes: (options.quotes ?? []).map((quote) => quote.text),
          attachment_ids: attachmentIDs,
          plan_requested: options.planRequested,
        })
        if (!prompt) return
        await mutateQueue({ op: 'append', message: prompt })
      })().catch(showQueueError)
      return
    }
    onSend(text, options)
  }, [mutateQueue, onSend, running, sessionId, showQueueError])

  const deletePrompt = useCallback((id: string) => {
    void mutateQueue({ op: 'delete', id }).catch(showQueueError)
  }, [mutateQueue, showQueueError])

  const editPrompt = useCallback((id: string, text: string) => {
    void mutateQueue({ op: 'edit', id, message: { text } }).catch(showQueueError)
  }, [mutateQueue, showQueueError])

  const reorderPrompts = useCallback((ids: string[]) => {
    void mutateQueue({ op: 'reorder', ids }).catch(showQueueError)
  }, [mutateQueue, showQueueError])

  const steerPrompt = useCallback((id: string) => {
    if (!session) return
    void (async () => {
      try {
        await mutateQueue({ op: 'steer', id })
        queryClient.invalidateQueries({ queryKey: keys.sessionMessages(sessionId) })
        queryClient.invalidateQueries({ queryKey: keys.sidebarSessions })
        queryClient.invalidateQueries({ queryKey: keys.allSessions })
      } catch (error) {
        queryClient.invalidateQueries({ queryKey: keys.sessionMessages(sessionId) })
        toast(`Couldn't steer prompt: ${(error as Error).message}`, 'danger')
      }
    })()
  }, [mutateQueue, queryClient, session, sessionId, toast])

  return {
    queuedPrompts,
    sessionRunning: running,
    steerDisabled: false,
    onSend: send,
    onSteerQueuedPrompt: steerPrompt,
    onDeleteQueuedPrompt: deletePrompt,
    onEditQueuedPrompt: editPrompt,
    onReorderQueuedPrompts: reorderPrompts,
  }
}

function isSessionRunning({
  session,
  acpState,
  streaming,
}: {
  session?: Session
  acpState?: string
  streaming: boolean
}) {
  return Boolean(
    streaming ||
      session?.status === 'running' ||
      (session?.runtime === 'acp' && ['running', 'starting'].includes(acpState ?? '')),
  )
}

function normalizeQueuedPrompts(prompts: QueuedMessage[]): QueuedMessage[] {
  return prompts.flatMap((prompt, index) => {
    const normalized = normalizeQueuedPrompt(prompt)
    if (!normalized) return []
    return [{ ...normalized, id: normalized.id?.trim() || `legacy-${index}` }]
  })
}

function normalizeQueuedPrompt(prompt: QueuedMessageInput): QueuedMessageInput | null {
  const text = prompt.text.trim()
  if (!text) return null
  const quotes = (prompt.quotes ?? []).map((quote) => quote.trim()).filter(Boolean)
  const attachmentIds = (prompt.attachment_ids ?? []).map((id) => id.trim()).filter(Boolean)
  return {
    ...(prompt.id?.trim() ? { id: prompt.id.trim() } : {}),
    text,
    ...(quotes.length ? { quotes } : {}),
    ...(attachmentIds.length ? { attachment_ids: attachmentIds } : {}),
    ...(prompt.plan_requested ? { plan_requested: true } : {}),
  }
}
