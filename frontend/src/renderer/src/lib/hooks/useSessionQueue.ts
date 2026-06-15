import { useQueryClient } from '@tanstack/react-query'
import { useCallback } from 'react'
import { useToast } from '@/components/ui/toast'
import { mutateSessionQueue, type QueueMutation, uploadSessionAttachment } from '@/lib/api/sessions'
import type { QueuedMessage, Session, SessionMessages } from '@/lib/api/types'
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
  const queuedPrompts = normalizeQueuedPrompts(session?.queued_messages ?? [])
  const running = isSessionRunning({ session, acpState, streaming })

  const mutateQueue = useCallback(async (mutation: QueueMutation, optimisticPrompts: QueuedMessage[]) => {
    const next = normalizeQueuedPrompts(optimisticPrompts)
    queryClient.setQueryData<SessionMessages>(keys.sessionMessages(sessionId), (prev) =>
      prev ? { ...prev, session: { ...prev.session, queued_messages: next } } : prev,
    )
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
  }, [queryClient, sessionId])

  const showQueueError = useCallback((error: unknown) => {
    toast(`Queue update failed: ${(error as Error).message}`, 'danger')
  }, [toast])

  const send = useCallback((text: string, options: SendMessageOptions = {}) => {
    if (running) {
      void (async () => {
        const attachments = options.files?.length
          ? await Promise.all(options.files.map((file) => uploadSessionAttachment(sessionId, file)))
          : []
        const prompt = normalizeQueuedPrompt({ text, attachment_ids: attachments.map((attachment) => attachment.id), plan_requested: options.planRequested })
        if (!prompt) return
        await mutateQueue(
          { op: 'append', message: prompt },
          [...queuedPrompts, prompt],
        )
      })().catch(showQueueError)
      return
    }
    onSend(text, options)
  }, [mutateQueue, onSend, queuedPrompts, running, sessionId, showQueueError])

  const deletePrompt = useCallback((index: number) => {
    void mutateQueue(
      { op: 'delete', index, expected: queuedPrompts[index]?.text },
      removeQueuedPrompt(queuedPrompts, index),
    ).catch(showQueueError)
  }, [mutateQueue, queuedPrompts, showQueueError])

  const editPrompt = useCallback((index: number, text: string) => {
    void mutateQueue(
      { op: 'edit', index, message: { text }, expected: queuedPrompts[index]?.text },
      queuedPrompts.map((prompt, i) => (i === index ? { ...prompt, text } : prompt)),
    ).catch(showQueueError)
  }, [mutateQueue, queuedPrompts, showQueueError])

  const movePrompt = useCallback((from: number, to: number) => {
    void mutateQueue(
      { op: 'move', from, to, expected: queuedPrompts[from]?.text },
      moveQueuedPrompt(queuedPrompts, from, to),
    ).catch(showQueueError)
  }, [mutateQueue, queuedPrompts, showQueueError])

  const steerPrompt = useCallback((index: number) => {
    const prompt = queuedPrompts[index]
    if (!prompt || !session) return
    if (running && session.runtime !== 'acp') return
    const nextQueue = removeQueuedPrompt(queuedPrompts, index)
    void (async () => {
      try {
        await mutateQueue({ op: 'steer', index, expected: prompt.text }, nextQueue)
        queryClient.invalidateQueries({ queryKey: keys.sessionMessages(sessionId) })
        queryClient.invalidateQueries({ queryKey: keys.sidebarSessions })
        queryClient.invalidateQueries({ queryKey: keys.allSessions })
      } catch (error) {
        queryClient.invalidateQueries({ queryKey: keys.sessionMessages(sessionId) })
        toast(`Couldn't steer prompt: ${(error as Error).message}`, 'danger')
      }
    })()
  }, [mutateQueue, queryClient, queuedPrompts, running, session, sessionId, toast])

  return {
    queuedPrompts,
    sessionRunning: running,
    steerDisabled: running && session?.runtime !== 'acp',
    onSend: send,
    onSteerQueuedPrompt: steerPrompt,
    onDeleteQueuedPrompt: deletePrompt,
    onEditQueuedPrompt: editPrompt,
    onMoveQueuedPrompt: movePrompt,
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
  return prompts.flatMap((prompt) => {
    const normalized = normalizeQueuedPrompt(prompt)
    return normalized ? [normalized] : []
  })
}

function normalizeQueuedPrompt(prompt: QueuedMessage): QueuedMessage | null {
  const text = prompt.text.trim()
  if (!text) return null
  const attachmentIds = (prompt.attachment_ids ?? []).map((id) => id.trim()).filter(Boolean)
  return {
    text,
    ...(attachmentIds.length ? { attachment_ids: attachmentIds } : {}),
    ...(prompt.plan_requested ? { plan_requested: true } : {}),
  }
}

function removeQueuedPrompt(prompts: QueuedMessage[], index: number): QueuedMessage[] {
  return prompts.filter((_, i) => i !== index)
}

function moveQueuedPrompt(prompts: QueuedMessage[], from: number, to: number): QueuedMessage[] {
  if (from === to || from < 0 || from >= prompts.length || to < 0 || to >= prompts.length) {
    return prompts
  }
  const next = [...prompts]
  const [item] = next.splice(from, 1)
  next.splice(to, 0, item)
  return next
}
