import { useQuery, useQueryClient } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { FileText, PanelRightClose, PanelRightOpen } from 'lucide-react'
import { motion } from 'motion/react'
import { useCallback, useEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { Composer, PlanDecisionDock } from '@/components/session/Composer'
import { MessageMarkdown } from '@/components/session/MessageMarkdown'
import { RepoActions } from '@/components/session/RepoActions'
import { SESSION_PANEL_WIDTH, SessionPanel } from '@/components/session/SessionPanel'
import { RuntimeBadge } from '@/components/sidebar/RuntimeBadge'
import { ThinkingBlock } from '@/components/session/ThinkingBlock'
import { TokenStats } from '@/components/session/TokenStats'
import { ToolCallCard } from '@/components/session/ToolCallCard'
import { Transcript } from '@/components/session/Transcript'
import { VoiceMode } from '@/components/session/VoiceMode'
import { isHiddenToolName } from '@/components/session/toolVisibility'
import { EmptyState } from '@/components/ui/EmptyState'
import { Skeleton, SkeletonRows } from '@/components/ui/Skeleton'
import {
  answerSessionInteractiveResponse,
  cancelSession,
  sessionMessagesQuery,
  uploadSessionAttachment,
} from '@/lib/api/sessions'
import { streamSessionMessage } from '@/lib/api/stream'
import type { ACPJobSnapshot, ACPPermission, ChatMessage, SessionEvent } from '@/lib/api/types'
import { useSessionEvents } from '@/lib/hooks/useSessionEvents'
import { useSessionQueue } from '@/lib/hooks/useSessionQueue'
import { takePendingMessage, takePendingVoice } from '@/lib/pendingMessage'
import { keys } from '@/lib/query/keys'
import { planSurfaceBelongsToSession, planSurfaceFromEvent } from '@/lib/planSurface'
import type { SendMessageOptions } from '@/lib/sendMessage'
import { coalesceSessionEvents } from '@/lib/sessionEvents'

export const Route = createFileRoute('/sessions/$sessionId')({
  component: SessionPage,
})

// One in-flight user → assistant exchange, rendered after the transcript
// while it streams; replaced by the refetched server history on completion.
interface LiveExchange {
  user: string
  at: string
  attachments: LiveAttachment[]
  reasoning: string
  assistant: string
  tools: { key: string; name: string; args?: string; result?: string }[]
  error?: string
}

interface LiveAttachment {
  id?: string
  name: string
  uri?: string
  mime_type?: string
  size?: number
  server_path?: string
  uploading?: boolean
}

function formatAttachmentSize(size?: number): string {
  if (!size) return ''
  if (size < 1024) return `${size} B`
  if (size < 1024 * 1024) return `${Math.round(size / 1024)} KB`
  return `${(size / (1024 * 1024)).toFixed(1)} MB`
}

function LiveAttachmentList({ attachments }: { attachments: LiveAttachment[] }) {
  if (!attachments.length) return null
  return (
    <div className="mt-2 flex flex-wrap gap-1">
      {attachments.map((attachment, index) => (
        <span
          key={attachment.id ?? `${attachment.name}-${index}`}
          className="inline-flex max-w-full items-center gap-1.5 rounded-full bg-bg px-2.5 py-1 text-xs text-ink-2"
        >
          <FileText size={13} className="shrink-0 text-primary" />
          <span className="max-w-[220px] truncate text-ink">{attachment.name}</span>
          <span className="shrink-0 text-ink-3">
            {attachment.uploading ? 'Uploading' : formatAttachmentSize(attachment.size)}
          </span>
        </span>
      ))}
    </div>
  )
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

// The chat column at its widest (max-w-[720px] + px-10 each side); the panel
// auto-shows only when it fits beside it — which is exactly when the sidebar
// hides or the window is wide enough.
const PANEL_CHAT_COMFORT = 800
const PANEL_PREF_KEY = 'jaz.sessionPanel'
type PanelPref = 'auto' | 'open' | 'closed'

function storedPanelPref(): PanelPref {
  const value = localStorage.getItem(PANEL_PREF_KEY)
  return value === 'open' || value === 'closed' ? value : 'auto'
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

  // Right panel: 'auto' follows available width; an explicit choice wins
  // until toggling lands back on what auto would do anyway.
  const [panelPref, setPanelPref] = useState<PanelPref>(storedPanelPref)
  const [pageWidth, setPageWidth] = useState(0)
  const panelObserver = useRef<ResizeObserver | null>(null)
  const measureRef = useCallback((el: HTMLDivElement | null) => {
    panelObserver.current?.disconnect()
    panelObserver.current = null
    if (!el) return
    const observer = new ResizeObserver(() => setPageWidth(el.clientWidth))
    observer.observe(el)
    setPageWidth(el.clientWidth)
    panelObserver.current = observer
  }, [])
  useEffect(() => {
    localStorage.setItem(PANEL_PREF_KEY, panelPref)
  }, [panelPref])

  const scrollRef = useRef<HTMLDivElement>(null)
  const nearBottom = useRef(true)
  const itemCount = (detail.data?.messages.length ?? 0) + events.data.length
  const liveSize = live
    ? live.reasoning.length +
      live.assistant.length +
      live.tools.length +
      live.attachments.length +
      (live.error?.length ?? 0)
    : 0

  // Stick to the bottom only when the reader is already there.
  useEffect(() => {
    const el = scrollRef.current
    if (el && nearBottom.current) el.scrollTop = el.scrollHeight
  }, [itemCount, liveSize])

  // Abandon an in-flight stream when leaving the session.
  useEffect(() => () => abortRef.current?.abort(), [sessionId])

  const handleSend = useCallback((text: string, options: SendMessageOptions = {}) => {
    const controller = new AbortController()
    const files = options.files ?? []
    abortRef.current = controller
    nearBottom.current = true
    setLive({
      user: text,
      at: new Date().toISOString(),
      attachments: files.map((file) => ({ name: file.name, size: file.size, uploading: true })),
      reasoning: '',
      assistant: '',
      tools: [],
    })
    setStreaming(true)
    streamingRef.current = true

    ;(async () => {
      const attachments = files.length
        ? await Promise.all(files.map((file) => uploadSessionAttachment(sessionId, file, controller.signal)))
        : []
      if (attachments.length) {
        setLive((prev) => (prev ? { ...prev, attachments } : prev))
      }
      await streamSessionMessage({
        sessionId,
        message: text,
        attachmentIds: attachments.map((attachment) => attachment.id),
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
              case 'tool_result': {
                if (isHiddenToolName(event.tool_name)) return prev
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
    })()
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
    handleSend(pending.text, { planRequested: pending.planRequested, files: pending.files ?? [] })
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
  const acpModesKnown = Boolean(
    acpModes?.plan_mode_id ||
      acpModes?.current_mode_id ||
      acpModes?.execution_mode_id ||
      acpModes?.available_modes?.length,
  )
  const planAvailable = session.runtime !== 'acp' || !acpModesKnown || Boolean(acpModes?.plan_mode_id)
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
  // The panel mirrors the transcript's notion of "current plan": the latest
  // plan-bearing event that belongs to this session.
  const panelPlanEvent = [...transcriptEvents]
    .reverse()
    .find((event) => planSurfaceFromEvent(event) && planSurfaceBelongsToSession(event, session.id))
  const panelPlan = panelPlanEvent ? planSurfaceFromEvent(panelPlanEvent) : undefined
  const hasPanelSpace = pageWidth >= PANEL_CHAT_COMFORT + SESSION_PANEL_WIDTH
  const panelOpen = panelPref === 'auto' ? hasPanelSpace : panelPref === 'open'
  const togglePanel = () => {
    const next = !panelOpen
    // Landing on what auto would do re-arms auto-show.
    setPanelPref(next === hasPanelSpace ? 'auto' : next ? 'open' : 'closed')
  }
  // Plan progress lives in the side panel, never in the thread; only a
  // proposed plan that needs the user's approval stays inline.
  const displayEvents = transcriptEvents.map((event) => {
    const surface = planSurfaceFromEvent(event)
    if (!surface || surface.awaitingApproval) return event
    if (event.acp) return { ...event, acp: { ...event.acp, plan: undefined } }
    return { ...event, plan: undefined }
  })
  const latestUserAt = Math.max(
    0,
    ...messages
      .filter((message) => message.role === 'user')
      .map((message) => Date.parse(message.created_at))
      .filter((time) => !Number.isNaN(time)),
  )
  // A failed native turn carries its error only on the session; ACP turns and
  // the in-flight live exchange already surface it inline, so showing the
  // banner too would duplicate it. When it's the only carrier, anchor it at
  // the bottom so it reads chronologically after the prompt that triggered it.
  const showErrorBanner =
    session.status === 'error' &&
    Boolean(session.error) &&
    !live?.error &&
    !displayEvents.some((event) => Boolean(event.acp?.error))
  const latestPlanDecisionEvent = [...transcriptEvents]
    .reverse()
    .find((event) => {
      const surface = planSurfaceFromEvent(event)
      return Boolean(
        surface?.awaitingApproval &&
          surface.approvalSessionId &&
          planSurfaceBelongsToSession(event, session.id),
      )
    })
  const latestPlanDecisionSurface = latestPlanDecisionEvent
    ? planSurfaceFromEvent(latestPlanDecisionEvent)
    : undefined
  const planDecisionSessionID = latestPlanDecisionSurface?.approvalSessionId
  const planDecisionAt = Date.parse(latestPlanDecisionEvent?.at ?? '')
  const planDecisionIsCurrent =
    !Number.isNaN(planDecisionAt) && planDecisionAt >= latestUserAt
  const showPlanDecision = Boolean(
    latestPlanDecisionSurface?.awaitingApproval &&
      planDecisionSessionID &&
      planDecisionIsCurrent &&
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
            blocks: [
              { type: 'text' as const, text: live.user },
              ...live.attachments.flatMap((attachment) =>
                attachment.id && attachment.uri
                  ? [
                      {
                        type: 'attachment' as const,
                        id: attachment.id,
                        name: attachment.name,
                        uri: attachment.uri,
                        mime_type: attachment.mime_type,
                        size: attachment.size,
                        server_path: attachment.server_path,
                      },
                    ]
                  : [],
              ),
            ],
            created_at: live.at,
          },
        ]
      : messages
  const titlebarSlot = document.getElementById('titlebar-slot')
  const titlebarActions = document.getElementById('titlebar-actions')

  if (voiceMode) {
    return <VoiceMode sessionId={sessionId} onExit={() => setVoiceMode(false)} />
  }

  return (
    <div ref={measureRef} className="flex h-full">
      {titlebarSlot
        ? createPortal(
            <>
              <RuntimeBadge session={session} truncate={false} />
              <TokenStats session={session} />
            </>,
            titlebarSlot,
          )
        : null}
      {titlebarActions
        ? createPortal(
            <>
              <RepoActions session={session} />
              <button
                type="button"
                aria-label={panelOpen ? 'Hide session panel' : 'Show session panel'}
                title={`${panelOpen ? 'Hide' : 'Show'} session panel`}
                onClick={togglePanel}
                className="grid size-8 cursor-pointer place-items-center rounded-full text-ink-2 transition-colors duration-200 hover:bg-surface-2 hover:text-ink"
              >
                {panelOpen ? <PanelRightClose size={16} /> : <PanelRightOpen size={16} />}
              </button>
            </>,
            titlebarActions,
          )
        : null}

      <div className="relative h-full min-w-0 flex-1">
        <div
          ref={scrollRef}
          className="h-full overflow-y-auto"
          onScroll={(e) => {
            const el = e.currentTarget
            nearBottom.current = el.scrollHeight - el.scrollTop - el.clientHeight < 80
          }}
        >
          <div className={`mx-auto max-w-[720px] px-10 pt-2 ${queue.queuedPrompts.length ? 'pb-72' : 'pb-40'}`}>
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
                          <LiveAttachmentList attachments={live.attachments} />
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
  
            {showErrorBanner ? (
              <p className="mt-5 max-w-[72ch] rounded-card bg-danger-soft px-3 py-2 text-sm text-danger select-text">
                {session.error}
              </p>
            ) : null}
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
                void sendACPFallback(planDecisionSessionID!, 'Implement the plan.', {
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
            fileRoot={session.runtime_ref?.cwd}
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

      {/* Docked, never overlapping: the chat pane flexes and stays centered
          between the sidebar and this panel. */}
      <motion.div
        className="h-full shrink-0 overflow-hidden"
        initial={false}
        animate={{ width: panelOpen ? SESSION_PANEL_WIDTH : 0 }}
        transition={{ type: 'spring', stiffness: 400, damping: 36 }}
      >
        <SessionPanel session={session} plan={panelPlan} working={sessionRunning} />
      </motion.div>
    </div>
  )
}
