import { useQuery, useQueryClient } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { motion } from 'motion/react'
import { useCallback, useEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { Composer, PlanDecisionDock } from '@/components/session/Composer'
import { MessageMarkdown } from '@/components/session/MessageMarkdown'
import { RuntimeBadge } from '@/components/sidebar/RuntimeBadge'
import { ThinkingBlock } from '@/components/session/ThinkingBlock'
import { ToolCallCard } from '@/components/session/ToolCallCard'
import { Transcript } from '@/components/session/Transcript'
import { VoiceMode } from '@/components/session/VoiceMode'
import { EmptyState } from '@/components/ui/EmptyState'
import { Skeleton, SkeletonRows } from '@/components/ui/Skeleton'
import {
  answerSessionInteractiveResponse,
  cancelSession,
  sessionMessagesQuery,
} from '@/lib/api/sessions'
import { streamSessionMessage } from '@/lib/api/stream'
import type { ACPJobSnapshot, ACPPermission, ChatMessage, SessionEvent } from '@/lib/api/types'
import { useSessionEvents } from '@/lib/hooks/useSessionEvents'
import { useSessionQueue } from '@/lib/hooks/useSessionQueue'
import { takePendingMessage, takePendingVoice } from '@/lib/pendingMessage'
import { keys } from '@/lib/query/keys'

export const Route = createFileRoute('/sessions/$sessionId')({
  component: SessionPage,
})

// One in-flight user → assistant exchange, rendered after the transcript
// while it streams; replaced by the refetched server history on completion.
interface LiveExchange {
  user: string
  at: string
  reasoning: string
  assistant: string
  tools: { key: string; name: string; args?: string; result?: string }[]
  error?: string
}

function acpSnapshotEvents(job: ACPJobSnapshot, pageSessionID: string): SessionEvent[] {
  if (job.parent_id === pageSessionID && job.parent_visible === false) return []
  const at = job.updated_at
  const events: SessionEvent[] = []
  if (
    job.assistant ||
    job.thought ||
    job.plan?.length ||
    job.tool_calls?.length ||
    job.error
  ) {
    events.push({
      session_id: pageSessionID,
      type: 'acp',
      at,
      content: job.assistant,
      acp: {
        id: job.id,
        slug: job.slug,
        title: job.title,
        parent_id: job.parent_id,
        agent: job.acp_agent,
        session_id: job.acp_session,
        state: job.state,
        stop_reason: job.stop_reason,
        assistant: job.assistant,
        thought: job.thought,
        error: job.error,
        modes: job.modes,
        plan: job.plan,
        tool_calls: job.tool_calls,
        permissions: job.permissions,
      },
    })
  }
  for (const permission of job.permissions ?? []) {
    if (!hasPermissionSurface(permission)) continue
    events.push({
      session_id: pageSessionID,
      type: 'permission_request',
      at,
      permission,
    })
  }
  return events
}

function hasPermissionSurface(permission: ACPPermission | undefined): boolean {
  if (!permission?.id?.trim()) return false
  return Boolean(
    permission.questions?.length ||
      permission.options?.length ||
      permission.locations?.length,
  )
}

function sanitizeParentChildACPEvent(event: SessionEvent, pageSessionID: string): SessionEvent | null {
  const acp = event.acp
  if (!acp || acp.parent_id !== pageSessionID || acp.id === pageSessionID) return event
  if (event.type === 'acp_message' || event.type === 'acp_thought' || event.type === 'acp_tool') {
    return null
  }
  return {
    ...event,
    content: undefined,
    acp: {
      ...acp,
      assistant: undefined,
      thought: undefined,
      tool_calls: undefined,
      permissions: undefined,
    },
  }
}

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

function coalesceSessionEvents(events: SessionEvent[]): SessionEvent[] {
  // Pass 1: dedupe by store-assigned seq (persisted history vs live SSE copy).
  const bySeq = new Map<string, number>()
  const deduped: SessionEvent[] = []
  for (const event of events) {
    if (!event.seq) {
      deduped.push(event)
      continue
    }
    const seqKey = `${event.session_id}:${event.seq}`
    const existing = bySeq.get(seqKey)
    if (existing === undefined) {
      bySeq.set(seqKey, deduped.length)
      deduped.push(event)
    } else {
      deduped[existing] = event
    }
  }
  // Pass 2: rolling state (status, plan, tool updates) keeps only its latest copy.
  const indexed = deduped.reduce<{ event: SessionEvent; index: number }[]>((acc, event, sourceIndex) => {
    const key = eventCoalesceKey(event)
    if (!key) return [...acc, { event, index: sourceIndex }]
    const index = acc.findIndex((item) => eventCoalesceKey(item.event) === key)
    if (index === -1) return [...acc, { event, index: sourceIndex }]
    const next = [...acc]
    next[index] = { event, index: next[index].index }
    return next
  }, [])
  return indexed
    .sort((a, b) => {
      const seqA = a.event.seq ?? 0
      const seqB = b.event.seq ?? 0
      if (seqA && seqB && a.event.session_id === b.event.session_id) {
        return seqA - seqB
      }
      const atA = Date.parse(a.event.at)
      const atB = Date.parse(b.event.at)
      const timeA = Number.isNaN(atA) ? 0 : atA
      const timeB = Number.isNaN(atB) ? 0 : atB
      return timeA - timeB || seqA - seqB || a.index - b.index
    })
    .map((item) => item.event)
}

function planHasActiveProgress(job: ACPJobSnapshot | NonNullable<SessionEvent['acp']>): boolean {
  return Boolean(
    job.plan?.some((entry) => ['in_progress', 'in-progress', 'running'].includes((entry.status ?? '').trim().toLowerCase())),
  )
}

function hasStoredAssistantMessage(messages: ChatMessage[], text?: string): boolean {
  const expected = text?.trim()
  if (!expected) return false
  return messages.some((message) => {
    if (message.role !== 'assistant') return false
    const blockText = message.blocks
      ?.filter((block) => block.type === 'text')
      .map((block) => (block.text ?? '').trim())
      .filter(Boolean)
      .join('\n\n')
    return message.content.trim() === expected || blockText === expected
  })
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
  const streamingRef = useRef(false)
  useSessionEvents(sessionId, streamingRef)

  const [live, setLive] = useState<LiveExchange | null>(null)
  const [streaming, setStreaming] = useState(false)
  const [voiceMode, setVoiceMode] = useState(false)
  const [planDecisionPending, setPlanDecisionPending] = useState(false)
  const [planDecisionError, setPlanDecisionError] = useState('')
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

  const handleSend = useCallback((text: string, options: { planRequested?: boolean } = {}) => {
    const controller = new AbortController()
    abortRef.current = controller
    nearBottom.current = true
    setLive({ user: text, at: new Date().toISOString(), reasoning: '', assistant: '', tools: [] })
    setStreaming(true)
    streamingRef.current = true

    streamSessionMessage({
      sessionId,
      message: text,
      planRequested: options.planRequested,
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
        streamingRef.current = false
        abortRef.current = null
        // The server persisted the exchange; swap the live view for history.
        await queryClient.refetchQueries({ queryKey: keys.sessionMessages(sessionId) })
        queryClient.invalidateQueries({ queryKey: keys.sidebarSessions })
        queryClient.invalidateQueries({ queryKey: keys.allSessions })
        setLive((prev) => (prev?.error ? prev : null))
      })
  }, [queryClient, sessionId])

  const sendACPFallback = useCallback(async (
    targetSessionID: string,
    text: string,
    options: { planRequested?: boolean; parentVisible?: boolean } = {},
  ) => {
    if (targetSessionID === sessionId) {
      handleSend(text, { planRequested: options.planRequested })
      return
    }
    await answerSessionInteractiveResponse(targetSessionID, {
      text,
      plan_requested: options.planRequested,
      parent_visible: options.parentVisible,
    })
    // Never invalidate sessionEvents: its queryFn returns [], wiping the SSE cache.
    queryClient.invalidateQueries({ queryKey: keys.sessionMessages(targetSessionID) })
    queryClient.invalidateQueries({ queryKey: keys.sidebarSessions })
    queryClient.invalidateQueries({ queryKey: keys.allSessions })
  }, [handleSend, queryClient, sessionId])

  const currentSession = detail.data?.session
  const queue = useSessionQueue({
    sessionId,
    session: currentSession,
    acpState: detail.data?.acp_state,
    streaming,
    onSend: handleSend,
  })

  // First message handed over from the New-session page. Wait for the session
  // detail query so StrictMode's initial effect cleanup cannot abort the send.
  useEffect(() => {
    if (!detail.isSuccess || sentPendingRef.current === sessionId) return
    const pending = takePendingMessage(sessionId)
    if (!pending) return
    sentPendingRef.current = sessionId
    handleSend(pending)
  }, [detail.isSuccess, handleSend, sessionId])

  // Voice button on /new: open this thread straight in voice mode.
  useEffect(() => {
    if (takePendingVoice(sessionId)) setVoiceMode(true)
  }, [sessionId])

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

  const {
    session,
    messages,
    acp_state: acpState,
    acp_assistant: acpAssistant,
    acp_thought: acpThought,
    acp_modes: acpModes,
    acp_plan: acpPlan,
    acp_tool_calls: acpToolCalls,
    acp_permissions: acpPermissions,
    acp_error: acpError,
    acp_children: acpChildren,
    events: persistedEvents = [],
  } = detail.data
  // The job snapshot is only a fallback: when events record the run, the
  // snapshot would repeat it and dump every tool call at the end.
  const eventsCoverOwnACP =
    [...persistedEvents, ...events.data].some(
      (event) =>
        (event.type === 'acp_message' || event.type === 'acp_thought' || event.type === 'acp_tool') &&
        event.acp?.id === session.id,
    ) || hasStoredAssistantMessage(messages, acpAssistant)
  const snapshotEvents: SessionEvent[] = [
    ...(session.runtime === 'acp'
      ? acpSnapshotEvents(
          {
            id: session.id,
            slug: session.slug,
            title: session.title,
            parent_id: session.parent_id,
            acp_agent: session.runtime_ref?.agent ?? 'acp',
            acp_session: session.runtime_ref?.session_id ?? '',
            state: acpState ?? session.status,
            assistant: eventsCoverOwnACP ? undefined : acpAssistant,
            thought: eventsCoverOwnACP ? undefined : acpThought,
            error: acpError,
            modes: acpModes,
            plan: acpPlan,
            tool_calls: eventsCoverOwnACP ? undefined : acpToolCalls,
            permissions: acpPermissions,
            updated_at: session.updated_at,
          },
          session.id,
        )
      : []),
    ...((acpChildren ?? []).flatMap((child) => acpSnapshotEvents(child, session.id))),
  ]
  const transcriptEvents = coalesceSessionEvents(
    [...persistedEvents, ...snapshotEvents, ...events.data].flatMap((event) => {
      // 'assistant' events are refresh signals; the message store has the content.
      if (event.type === 'assistant') return []
      // Old rows round-tripped a typed-nil ACP into an empty struct.
      if (event.acp && !event.acp.id) event = { ...event, acp: undefined }
      const sanitized = sanitizeParentChildACPEvent(event, session.id)
      return sanitized ? [sanitized] : []
    }),
  )
  const planAvailable = session.runtime === 'acp' && Boolean(acpModes?.plan_mode_id)
  const pendingPermissionEvents = new Map<string, SessionEvent>()
  for (const event of transcriptEvents) {
    if (event.type === 'permission_request' && event.permission?.id) {
      if (hasPermissionSurface(event.permission)) {
        pendingPermissionEvents.set(event.permission.id, event)
      }
    }
    if (event.type === 'permission_response' && event.permission?.id) {
      pendingPermissionEvents.delete(event.permission.id)
    }
  }
  const activePermissionEvent = [...pendingPermissionEvents.values()].at(-1)
  const hasPendingPermission = Boolean(activePermissionEvent)
  const displayEvents = transcriptEvents
  const latestPlanDecisionEvent = [...transcriptEvents]
    .reverse()
    .find((event) => {
      const acp = event.acp
      const modes = acp?.modes
      if (!acp || !modes?.plan_mode_id || modes.current_mode_id !== modes.plan_mode_id) return false
      if (acp.state === 'running') return false
      if (planHasActiveProgress(acp)) return false
      const belongsToPage = acp.id === session.id || acp.parent_id === session.id
      return belongsToPage && Boolean(acp.plan?.length)
    })
  const directPlanModes = latestPlanDecisionEvent?.acp?.modes ?? acpModes
  const directPlanAwaitingDecision = Boolean(
    latestPlanDecisionEvent &&
      directPlanModes?.plan_mode_id &&
      directPlanModes.current_mode_id === directPlanModes.plan_mode_id,
  )
  const planDecisionSessionID = latestPlanDecisionEvent?.acp?.id
  const showPlanDecision = Boolean(
    directPlanAwaitingDecision &&
      planDecisionSessionID &&
      !streaming &&
      !live &&
      !hasPendingPermission,
  )
  const empty = messages.length === 0 && displayEvents.length === 0 && !live
  const isACP = session.runtime === 'acp'
  // Covers turns started elsewhere (parent-triggered, or refresh mid-turn).
  const sessionRunning = queue.sessionRunning
  // ACP turns stream through events; the live exchange only contributes the
  // not-yet-refetched user bubble, injected so mid-turn events sort after it.
  const lastUserMessage = [...messages].reverse().find((message) => message.role === 'user')
  const transcriptMessages =
    isACP && live && lastUserMessage?.content.trim() !== live.user.trim()
      ? [
          ...messages,
          {
            seq: (messages.at(-1)?.seq ?? 0) + 1_000_000,
            role: 'user' as const,
            content: live.user,
            blocks: [{ type: 'text' as const, text: live.user }],
            created_at: live.at,
          },
        ]
      : messages
  const titlebarSlot = document.getElementById('titlebar-slot')

  if (voiceMode) {
    return <VoiceMode sessionId={sessionId} onExit={() => setVoiceMode(false)} />
  }

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
        <div className={`mx-auto max-w-[720px] px-10 pt-2 ${queue.queuedPrompts.length ? 'pb-72' : 'pb-40'}`}>
          {session.status === 'error' && session.error ? (
            <p className="mb-5 max-w-[72ch] rounded-card bg-danger-soft px-3 py-2 text-sm text-danger select-text">
              {session.error}
            </p>
          ) : null}

          {empty ? (
            <EmptyState title="Start the conversation">
              <p>Messages stream in live as your assistant thinks and works.</p>
            </EmptyState>
          ) : (
            <Transcript
              messages={transcriptMessages}
              events={displayEvents}
              sessionId={session.id}
              groupTurns={isACP}
              working={sessionRunning}
              tail={
                live && !isACP ? (
                  <div className="flex flex-col gap-5">
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
                ) : live?.error && isACP ? (
                  <p className="max-w-[72ch] rounded-card bg-danger-soft px-3 py-2 text-sm text-danger select-text">
                    {live.error}
                  </p>
                ) : null
              }
            />
          )}

        </div>
      </div>

      {showPlanDecision ? (
        <>
          {planDecisionError ? (
            <p className="absolute inset-x-0 bottom-32 mx-auto max-w-[640px] rounded-card bg-danger-soft px-3 py-2 text-sm text-danger select-text">
              {planDecisionError}
            </p>
          ) : null}
          <PlanDecisionDock
            pending={planDecisionPending}
            onImplement={() => {
              setPlanDecisionPending(true)
              setPlanDecisionError('')
              void sendACPFallback(planDecisionSessionID!, 'Approved. Proceed with the plan.', {
                parentVisible: planDecisionSessionID !== session.id,
              })
                .catch((err: Error) => setPlanDecisionError(err.message || 'Sending the approval failed.'))
                .finally(() => setPlanDecisionPending(false))
            }}
            onClarify={(text) => {
              setPlanDecisionPending(true)
              setPlanDecisionError('')
              void sendACPFallback(planDecisionSessionID!, text, {
                planRequested: true,
                parentVisible: planDecisionSessionID !== session.id,
              })
                .catch((err: Error) => setPlanDecisionError(err.message || 'Sending the reply failed.'))
                .finally(() => setPlanDecisionPending(false))
            }}
          />
        </>
      ) : (
        <Composer
          streaming={sessionRunning}
          planAvailable={planAvailable}
          queuedPrompts={queue.queuedPrompts}
          steerDisabled={queue.steerDisabled}
          onSend={queue.onSend}
          onStop={() => {
            // the turn runs detached server-side; stop it there first
            void cancelSession(sessionId).catch(() => {})
            abortRef.current?.abort()
          }}
          onVoice={session.runtime !== 'acp' ? () => setVoiceMode(true) : undefined}
          onSteerQueuedPrompt={queue.onSteerQueuedPrompt}
          onDeleteQueuedPrompt={queue.onDeleteQueuedPrompt}
          onEditQueuedPrompt={queue.onEditQueuedPrompt}
          onMoveQueuedPrompt={queue.onMoveQueuedPrompt}
        />
      )}
    </div>
  )
}
