import { useQueryClient } from '@tanstack/react-query'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { isCloudApiSession } from '@/lib/api/client'
import type { AgentStreamEvent } from '@/lib/api/stream'
import type { ChatMessage, SessionMessages } from '@/lib/api/types'
import { deliverMessage, newOutboxSend } from '@/lib/messageDelivery'
import {
  outbox,
  outboxTranscript,
  removeOutboxMessages,
  useOutbox,
  type OutboxSend,
  type PendingUserMessage,
} from '@/lib/outbox'
import { invalidateSessionTurnQueries } from '@/lib/query/invalidate'
import { keys } from '@/lib/query/keys'
import type { SendMessageOptions } from '@/lib/sendMessage'
import { hasCompletedLiveReply, type LiveExchange } from './liveTranscript'
import { isHiddenToolName } from './toolVisibility'

const NO_MESSAGES: ChatMessage[] = []

// Sends a thread's messages and reads them back: `messages` is the loaded
// history with every message still on its way to it, or refused, in place.
export function useLiveSessionSend({
  sessionId,
  onCriticalError,
  history,
  initialPrompt,
}: {
  sessionId: string
  onCriticalError: (message: string) => void
  history?: SessionMessages
  // The message a new thread was started with, already accepted.
  initialPrompt?: PendingUserMessage
}) {
  const queryClient = useQueryClient()
  const [live, setLive] = useState<LiveExchange | null>(null)
  const [streaming, setStreaming] = useState(false)
  const sendRef = useRef<{ controller: AbortController; accepted: Promise<boolean> } | null>(null)
  const recovered = Boolean(live?.error && history && hasCompletedLiveReply(live, history))
  const sends = useOutbox(sessionId)
  const transcript = useMemo(
    () => outboxTranscript(
      history?.messages ?? NO_MESSAGES,
      initialPrompt ? [{ ...initialPrompt, id: 'initial', state: 'delivered' }, ...sends] : sends,
    ),
    [history?.messages, initialPrompt, sends],
  )

  useEffect(() => removeOutboxMessages(sessionId, transcript.settled), [sessionId, transcript])

  useEffect(() => {
    if (recovered) setLive((current) => (current === live ? null : current))
  }, [history, live, recovered])

  useEffect(() => () => {
    const pending = sendRef.current
    if (pending) {
      void pending.accepted.then(() => pending.controller.abort())
    }
  }, [sessionId])

  const deliver = useCallback((entry: OutboxSend) => {
    const controller = new AbortController()
    const current = () => sendRef.current?.controller === controller
    const isCloud = history?.session.backend === 'cloud' || isCloudApiSession()
    setLive({
      user: entry.text,
      at: entry.message.created_at,
      baselineMessageSeq: entry.baselineMessageSeq,
      baselineEventSeq: queryClient.getQueryData<SessionMessages>(keys.sessionMessages(sessionId))?.latest_event_seq ?? 0,
      planRequested: Boolean(entry.options.planRequested),
      assistant: '',
      tools: [],
    })
    setStreaming(true)
    const delivery = deliverMessage({
      queryClient,
      sessionId,
      entry,
      cloudRunId: isCloud ? history?.session.run_id || sessionId : undefined,
      signal: controller.signal,
      onEvent: (event) => {
        if (current()) setLive((prev) => (prev ? mergeLiveStreamEvent(prev, event) : prev))
      },
    })
    sendRef.current = { controller, accepted: delivery.accepted }

    void delivery.finished
      .catch(async (err: Error) => {
        if (controller.signal.aborted) return
        // A refused message says so on its own bubble; an accepted one whose
        // turn then failed is the thread's error.
        const accepted = await delivery.accepted
        if (accepted && !current()) return
        onCriticalError(err.message || 'Something went wrong.')
        if (current()) setLive((prev) => (prev && accepted ? { ...prev, error: err.message } : null))
      })
      .finally(async () => {
        await delivery.settled
        queryClient.invalidateQueries({ queryKey: keys.sidebarSessions })
        invalidateSessionTurnQueries(queryClient, sessionId, isCloud)
        if (current()) {
          setStreaming(false)
          sendRef.current = null
          setLive((prev) => (prev?.error ? prev : null))
        }
      })
  }, [history?.session.backend, history?.session.run_id, onCriticalError, queryClient, sessionId])

  const send = useCallback((text: string, options: SendMessageOptions = {}) => {
    const loaded = queryClient.getQueryData<SessionMessages>(keys.sessionMessages(sessionId))
    deliver(newOutboxSend(text, options, loaded?.messages.at(-1)?.seq ?? 0))
  }, [deliver, queryClient, sessionId])

  const retry = useCallback((id: string) => {
    const failed = outbox(sessionId).find((item) => item.id === id && item.state === 'failed')
    if (failed) deliver(failed)
  }, [deliver, sessionId])

  const discard = useCallback((id: string) => removeOutboxMessages(sessionId, [id]), [sessionId])

  const abort = useCallback(() => {
    const pending = sendRef.current
    if (!pending) {
      return
    }
    pending.controller.abort()
    sendRef.current = null
    setStreaming(false)
    setLive(null)
  }, [])

  return { live: recovered ? null : live, streaming, messages: transcript.messages, send, retry, discard, abort }
}

function mergeLiveStreamEvent(prev: LiveExchange, event: AgentStreamEvent): LiveExchange {
  switch (event.type) {
    case 'delta':
      return { ...prev, assistant: prev.assistant + (event.delta ?? '') }
    case 'tool_call':
      return addLiveToolCall(prev, event)
    case 'tool_result':
      return addLiveToolResult(prev, event)
    case 'error':
      return { ...prev, error: event.error || 'Something went wrong.' }
    default:
      return prev
  }
}

function addLiveToolCall(prev: LiveExchange, event: AgentStreamEvent): LiveExchange {
  const name = event.tool_call?.function?.name ?? event.tool_name ?? 'tool'
  if (isHiddenToolName(name)) return prev
  return {
    ...prev,
    tools: [
      ...prev.tools,
      {
        key: event.tool_call?.id ?? `${name}-${prev.tools.length}`,
        name,
        args: event.tool_call?.function?.arguments,
      },
    ],
  }
}

function addLiveToolResult(prev: LiveExchange, event: AgentStreamEvent): LiveExchange {
  if (isHiddenToolName(event.tool_name)) return prev
  const idx = prev.tools.findLastIndex((tool) => tool.result === undefined)
  const tools =
    idx === -1
      ? [
          ...prev.tools,
          {
            key: `result-${prev.tools.length}`,
            name: event.tool_name ?? 'tool',
            result: event.result,
          },
        ]
      : prev.tools.map((tool, index) => (index === idx ? { ...tool, result: event.result } : tool))
  return { ...prev, tools }
}
