import { useQuery, useQueryClient } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { motion } from 'motion/react'
import { useCallback, useEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { Composer } from '@/components/session/Composer'
import { MessageMarkdown } from '@/components/session/MessageMarkdown'
import { RuntimeBadge } from '@/components/sidebar/RuntimeBadge'
import { ThinkingBlock } from '@/components/session/ThinkingBlock'
import { ToolCallCard } from '@/components/session/ToolCallCard'
import { Transcript } from '@/components/session/Transcript'
import { EmptyState } from '@/components/ui/EmptyState'
import { Skeleton, SkeletonRows } from '@/components/ui/Skeleton'
import { sessionMessagesQuery } from '@/lib/api/sessions'
import { streamSessionMessage } from '@/lib/api/stream'
import type { SessionEvent } from '@/lib/api/types'
import { useSessionEvents } from '@/lib/hooks/useSessionEvents'
import { takePendingMessage } from '@/lib/pendingMessage'
import { keys } from '@/lib/query/keys'

export const Route = createFileRoute('/sessions/$sessionId')({
  component: SessionPage,
})

// One in-flight user → assistant exchange, rendered after the transcript
// while it streams; replaced by the refetched server history on completion.
interface LiveExchange {
  user: string
  reasoning: string
  assistant: string
  tools: { key: string; name: string; args?: string; result?: string }[]
  error?: string
}

function SessionPage() {
  const { sessionId } = Route.useParams()
  const queryClient = useQueryClient()
  const detail = useQuery(sessionMessagesQuery(sessionId))
  const events = useQuery<SessionEvent[]>({
    queryKey: keys.sessionEvents(sessionId),
    queryFn: () => [],
    initialData: [],
    staleTime: Infinity,
    gcTime: Infinity,
  })
  useSessionEvents(sessionId)

  const [live, setLive] = useState<LiveExchange | null>(null)
  const [streaming, setStreaming] = useState(false)
  const abortRef = useRef<AbortController | null>(null)
  const sentPendingRef = useRef<string | null>(null)

  const scrollRef = useRef<HTMLDivElement>(null)
  const nearBottom = useRef(true)
  const itemCount = (detail.data?.messages.length ?? 0) + events.data.length
  const liveSize = live
    ? live.reasoning.length + live.assistant.length + live.tools.length + (live.error?.length ?? 0)
    : 0

  // Stick to the bottom only when the reader is already there.
  useEffect(() => {
    const el = scrollRef.current
    if (el && nearBottom.current) el.scrollTop = el.scrollHeight
  }, [itemCount, liveSize])

  // Abandon an in-flight stream when leaving the session.
  useEffect(() => () => abortRef.current?.abort(), [sessionId])

  const handleSend = useCallback((text: string) => {
    const controller = new AbortController()
    abortRef.current = controller
    nearBottom.current = true
    setLive({ user: text, reasoning: '', assistant: '', tools: [] })
    setStreaming(true)

    streamSessionMessage({
      sessionId,
      message: text,
      signal: controller.signal,
      onEvent: (event) => {
        setLive((prev) => {
          if (!prev) return prev
          switch (event.type) {
            case 'delta':
              return { ...prev, assistant: prev.assistant + (event.delta ?? '') }
            case 'reasoning':
              return { ...prev, reasoning: prev.reasoning + (event.reasoning ?? '') }
            case 'tool_call': {
              const name = event.tool_call?.function?.name ?? event.tool_name ?? 'tool'
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
            case 'tool_result': {
              const idx = prev.tools.findLastIndex((t) => t.result === undefined)
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
                  : prev.tools.map((t, i) => (i === idx ? { ...t, result: event.result } : t))
              return { ...prev, tools }
            }
            case 'error':
              return { ...prev, error: event.error || 'Something went wrong.' }
            default:
              return prev
          }
        })
      },
    })
      .catch((err: Error) => {
        if (controller.signal.aborted) return
        setLive((prev) => (prev ? { ...prev, error: err.message } : prev))
      })
      .finally(async () => {
        setStreaming(false)
        abortRef.current = null
        // The server persisted the exchange; swap the live view for history.
        await queryClient.refetchQueries({ queryKey: keys.sessionMessages(sessionId) })
        queryClient.invalidateQueries({ queryKey: keys.sidebarSessions })
        queryClient.invalidateQueries({ queryKey: keys.allSessions })
        setLive((prev) => (prev?.error ? prev : null))
      })
  }, [queryClient, sessionId])

  // First message handed over from the New-session page. Wait for the session
  // detail query so StrictMode's initial effect cleanup cannot abort the send.
  useEffect(() => {
    if (!detail.isSuccess || sentPendingRef.current === sessionId) return
    const pending = takePendingMessage(sessionId)
    if (!pending) return
    sentPendingRef.current = sessionId
    handleSend(pending)
  }, [detail.isSuccess, handleSend, sessionId])

  if (detail.isPending) {
    return (
      <div className="mx-auto max-w-[720px] px-10">
        <Skeleton className="mb-6 h-7 w-64" />
        <SkeletonRows count={5} />
      </div>
    )
  }

  if (detail.isError) {
    return (
      <EmptyState title="Couldn't load this session">
        <p>{detail.error.message}</p>
      </EmptyState>
    )
  }

  const { session, messages, acp_state: acpState } = detail.data
  const inactiveACP = session.runtime === 'acp' && acpState === 'not_running'
  const empty = messages.length === 0 && events.data.length === 0 && !live
  const titlebarSlot = document.getElementById('titlebar-slot')

  return (
    <div className="relative h-full">
      {/* runtime tag for ACP threads, shown in the titlebar next to the
          sidebar toggle: which agent runs this thread (codex, …) */}
      {session.runtime === 'acp' && titlebarSlot
        ? createPortal(<RuntimeBadge session={session} />, titlebarSlot)
        : null}

      <div
        ref={scrollRef}
        className="h-full overflow-y-auto"
        onScroll={(e) => {
          const el = e.currentTarget
          nearBottom.current = el.scrollHeight - el.scrollTop - el.clientHeight < 80
        }}
      >
        <div className="mx-auto max-w-[720px] px-10 pt-2 pb-40">
          {empty ? (
            <EmptyState title="Start the conversation">
              <p>Messages stream in live as your assistant thinks and works.</p>
            </EmptyState>
          ) : (
            <Transcript messages={messages} events={events.data} />
          )}

          {live ? (
            <div className="flex flex-col gap-5 pt-5">
              <motion.div
                className="flex justify-end"
                initial={{ opacity: 0, y: 8 }}
                animate={{ opacity: 1, y: 0 }}
                transition={{ type: 'spring', stiffness: 380, damping: 30 }}
              >
                <div className="max-w-[80%] rounded-card bg-surface px-3.5 py-2.5 text-sm whitespace-pre-wrap select-text">
                  {live.user}
                </div>
              </motion.div>
              <ThinkingBlock text={live.reasoning} pending={streaming} />
              {live.tools.map((tool) => (
                <ToolCallCard
                  key={tool.key}
                  name={tool.name}
                  args={tool.args}
                  result={tool.result}
                  pending={streaming && tool.result === undefined}
                />
              ))}
              {live.assistant ? (
                <MessageMarkdown text={live.assistant} />
              ) : streaming ? (
                <p className="animate-pulse text-sm text-ink-3">Thinking…</p>
              ) : null}
              {live.error ? (
                <p className="max-w-[72ch] rounded-card bg-danger-soft px-3 py-2 text-sm text-danger select-text">
                  {live.error}
                </p>
              ) : null}
            </div>
          ) : null}

        </div>
      </div>

      <Composer
        streaming={streaming}
        disabled={inactiveACP}
        placeholder={inactiveACP ? 'ACP session is not active' : undefined}
        onSend={handleSend}
        onStop={() => abortRef.current?.abort()}
      />
    </div>
  )
}
