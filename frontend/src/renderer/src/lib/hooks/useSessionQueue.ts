import { useQueryClient } from '@tanstack/react-query'
import { useCallback, useEffect, useRef } from 'react'
import { useToast } from '@/components/ui/toast'
import { mutateSessionQueue, type QueueMutation, uploadSessionAttachment } from '@/lib/api/sessions'
import type { QueuedMessage, QueuedMessageInput, Session, SessionMessages } from '@/lib/api/types'
import { keys } from '@/lib/query/keys'
import { preparedSendMessage, type SendMessageHandler, type SendMessageOptions } from '@/lib/sendMessage'
import { normalizeBrowserAnnotation } from '@/lib/messageContext'

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
  onSend: SendMessageHandler
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
      return (async () => {
        const uploaded = options.files?.length
          ? await Promise.all(options.files.map((file) => uploadSessionAttachment(sessionId, file)))
          : []
        const prepared = preparedSendMessage(options, uploaded)
        const prompt = normalizeQueuedPrompt({
          text,
          contexts: prepared.contexts,
          attachment_ids: prepared.attachmentIds,
          plan_requested: options.planRequested,
          goal_requested: options.goalRequested,
        })
        if (!prompt) return
        await mutateQueue({ op: 'append', message: prompt })
      })().catch((error) => {
        showQueueError(error)
        throw error
      })
    }
    return onSend(text, options)
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
  const contexts = normalizeContexts(prompt.contexts)
  const legacyQuotes = (prompt.quotes ?? []).map((quote) => quote.trim()).filter(Boolean)
  const attachmentIds = (prompt.attachment_ids ?? []).map((id) => id.trim()).filter(Boolean)
  if (!text && contexts.length === 0 && legacyQuotes.length === 0 && attachmentIds.length === 0) {
    return null
  }
  return {
    ...(prompt.id?.trim() ? { id: prompt.id.trim() } : {}),
    text,
    ...(contexts.length || legacyQuotes.length
      ? { contexts: [...legacyQuotes.map((text) => ({ type: 'selection' as const, text })), ...contexts] }
      : {}),
    ...(attachmentIds.length ? { attachment_ids: attachmentIds } : {}),
    ...(prompt.plan_requested ? { plan_requested: true } : {}),
    ...(prompt.goal_requested ? { goal_requested: true } : {}),
  }
}

function normalizeContexts(contexts: QueuedMessageInput['contexts'] = []): NonNullable<QueuedMessageInput['contexts']> {
  return contexts.flatMap<NonNullable<QueuedMessageInput['contexts']>[number]>((context) => {
    if (context.type === 'selection') {
      const text = context.text?.trim()
      if (!text) return []
      return [{ type: 'selection' as const, text, comment: context.comment?.trim() || undefined }]
    }
    if (context.type !== 'browser_annotation' || !context.browser_annotation) return []
    const annotation = normalizeBrowserAnnotation(context.browser_annotation)
    return annotation ? [{ type: 'browser_annotation' as const, browser_annotation: annotation }] : []
  })
}
