import { useQuery, useQueryClient } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { ArrowDown } from 'lucide-react'
import { AnimatePresence, motion } from 'motion/react'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { BottomDock } from '@/components/session/BottomDock'
import { Composer, PlanDecisionCard } from '@/components/session/Composer'
import { LiveAttachmentList } from '@/components/session/LiveAttachmentList'
import { MessageContexts } from '@/components/session/MessageContexts'
import { SelectionContextToolbar } from '@/components/session/SelectionContextToolbar'
import { useComposerContexts } from '@/components/session/useComposerContexts'
import { FileReaderLinkProvider, MessageMarkdown, PreviewLinkProvider } from '@/components/session/MessageMarkdown'
import { MentionText } from '@/components/session/mentions'
import { SessionErrorNotice } from '@/components/session/SessionErrorNotice'
import { SessionLivenessIndicator } from '@/components/session/SessionLivenessIndicator'
import { PendingSteerBubble } from '@/components/session/PendingSteerBubble'
import { SidePanel, type SidePanelView } from '@/components/session/SidePanel'
import { SidePanelControl, useSidePanelState } from '@/components/session/SidePanelState'
import { RuntimeBadge } from '@/components/sidebar/RuntimeBadge'
import { ArtifactBlock } from '@/components/session/ArtifactBlock'
import { ThinkingBlock } from '@/components/session/ThinkingBlock'
import { ThreadFindBar } from '@/components/session/ThreadFindBar'
import { TokenStats } from '@/components/session/TokenStats'
import { ToolCallCard } from '@/components/session/ToolCallCard'
import { Transcript } from '@/components/session/Transcript'
import { THREAD_COLUMN_CLASS } from '@/components/session/threadLayout'
import { isArtifactToolName } from '@/components/session/toolVisibility'
import { useThreadFind } from '@/components/session/useThreadFind'
import { useThreadAutoScroll } from '@/components/session/useThreadAutoScroll'
import { liveExchangeSize, liveUserMessage, useLiveSessionSend } from '@/components/session/useLiveSessionSend'
import { EmptyState } from '@/components/ui/EmptyState'
import { FileDropScope } from '@/components/ui/FileDrop'
import { Skeleton, SkeletonRows } from '@/components/ui/Skeleton'
import { useToast } from '@/components/ui/toast'
import {
  answerSessionInteractiveResponse,
  cancelSession,
  sendSessionSideChat,
  sessionMessagesQuery,
  sessionRepoQuery,
  uploadSessionAttachment,
} from '@/lib/api/sessions'
import type { ACPJobSnapshot, ACPModeState, ChatMessage, Session, SessionEvent, SessionMessages } from '@/lib/api/types'
import { drawerSlide } from '@/lib/dom/drawer'
import { useIsMobile } from '@/lib/hooks/useIsMobile'
import { useSessionEvents } from '@/lib/hooks/useSessionEvents'
import { useSessionQueue } from '@/lib/hooks/useSessionQueue'
import { takePendingMessage } from '@/lib/pendingMessage'
import { keys } from '@/lib/query/keys'
import { providerSubagentsFromEvents } from '@/lib/providerSubagents'
import { spawnedThreadsFromSources } from '@/lib/spawnedThreads'
import {
  approvalPlanSurfaceFromEvent,
  hasProgressSignal,
  progressSurfaceFromEvent,
  taskSurfaceBelongsToSession,
} from '@/lib/taskSurface'
import { preparedSendMessage, type SendMessageOptions } from '@/lib/sendMessage'
import { coalesceSessionEvents, sessionEventPlacement } from '@/lib/sessionEvents'
import { activePermissionIDs, isPermissionAwaitingResponse, resolveInactivePermissions } from '@/lib/sessionPermissions'
import { latestEventTimeISO } from '@/lib/sessionLiveness'
import { useTitlebarActions, useTitlebarSlot } from '@/lib/titlebar'

type SessionSearch = {
  message?: number
}

export const Route = createFileRoute('/sessions/$sessionId')({
  validateSearch: (search): SessionSearch => {
    const raw = search.message
    const message = typeof raw === 'number' ? raw : typeof raw === 'string' ? Number(raw) : 0
    return Number.isSafeInteger(message) && message > 0 ? { message } : {}
  },
  component: SessionRoute,
})

function SessionRoute() {
  const { sessionId } = Route.useParams()
  const search = Route.useSearch()
  return <SessionPage key={sessionId} sessionId={sessionId} search={search} />
}

function SessionTitlebar({
  session,
  isMobile,
  sidePanelOpen,
  sidePanelView,
  sideChatAvailable,
  fileAvailable,
  onToggleSidePanel,
  onSelectSidePanelView,
}: {
  session: Session
  isMobile: boolean
  sidePanelOpen: boolean
  sidePanelView: SidePanelView
  sideChatAvailable: boolean
  fileAvailable: boolean
  onToggleSidePanel: () => void
  onSelectSidePanelView: (view: SidePanelView) => void
}) {
  const slot = useMemo(
    () => (
      <>
        <RuntimeBadge session={session} truncate={isMobile} />
        <TokenStats session={session} />
      </>
    ),
    [isMobile, session],
  )
  useTitlebarSlot(slot)

  const actions = useMemo(
    () => (
      <SidePanelControl
        open={sidePanelOpen}
        view={sidePanelView}
        sideChatAvailable={sideChatAvailable}
        fileAvailable={fileAvailable}
        onToggle={onToggleSidePanel}
        onSelectView={onSelectSidePanelView}
      />
    ),
    [
      fileAvailable,
      onSelectSidePanelView,
      onToggleSidePanel,
      sideChatAvailable,
      sidePanelOpen,
      sidePanelView,
    ],
  )
  useTitlebarActions(actions)

  return null
}

function isCodexACPSession(session: Session | undefined): boolean {
  return session?.runtime === 'acp' && session.runtime_ref?.agent?.trim().toLowerCase() === 'codex'
}

function stripACPError(event: SessionEvent): SessionEvent {
  if (!event.acp?.error) return event
  return { ...event, acp: { ...event.acp, error: undefined } }
}

function stripProgressSignal(event: SessionEvent): SessionEvent {
  if (event.acp) {
    const { plan: _plan, ...acp } = event.acp
    return { ...event, acp }
  }
  const { plan: _plan, ...out } = event
  return out
}

function sessionEventErrorMessage(event: SessionEvent): string {
  return event.acp?.error?.trim() ?? ''
}

function modeStateKnown(modes?: ACPModeState): boolean {
  return Boolean(
    modes?.plan_mode_id ||
      modes?.current_mode_id ||
      modes?.available_modes?.length,
  )
}

function planModeActive(modes?: ACPModeState): boolean {
  return Boolean(modes?.plan_mode_id && modes.current_mode_id === modes.plan_mode_id)
}

function latestACPModeState(sessionId: string, events: SessionEvent[]): ACPModeState | undefined {
  let latest: ACPModeState | undefined
  for (const event of events) {
    if (event.acp?.id !== sessionId || !modeStateKnown(event.acp.modes)) continue
    latest = event.acp.modes
  }
  return latest
}

function ScrollToBottomButton({ visible, onClick }: { visible: boolean; onClick: () => void }) {
  return (
    <AnimatePresence initial={false}>
      {visible ? (
        <motion.button
          type="button"
          key="scroll-to-bottom"
          aria-label="Scroll to latest message"
          title="Scroll to latest message"
          onClick={onClick}
          initial={{ opacity: 0, scale: 0.85, y: 6 }}
          animate={{ opacity: 1, scale: 1, y: 0 }}
          exit={{ opacity: 0, scale: 0.85, y: 6 }}
          whileTap={{ scale: 0.96 }}
          transition={{ type: 'spring', duration: 0.3, bounce: 0 }}
          className="mx-auto mb-2 grid size-10 place-items-center rounded-full bg-surface text-ink shadow-[0_8px_24px_rgba(0,0,0,0.14)] transition-colors duration-150 hover:bg-surface-2"
        >
          <ArrowDown size={17} />
        </motion.button>
      ) : null}
    </AnimatePresence>
  )
}

function acpSnapshotEvents(job: ACPJobSnapshot): SessionEvent[] {
  const at = job.last_event_at || job.updated_at
  const events: SessionEvent[] = []
  if (
    job.assistant ||
    job.thought ||
    job.plan?.length ||
    job.tool_calls?.length ||
    job.error
  ) {
    events.push({
      session_id: job.id,
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
        model_provider: job.model_provider,
        model: job.model,
        reasoning_effort: job.reasoning_effort,
        state: job.state,
        stop_reason: job.stop_reason,
        assistant: job.assistant,
        thought: job.thought,
        error: job.error,
        modes: job.modes,
        plan: job.plan,
        tool_calls: job.tool_calls,
        permissions: job.permissions,
        last_event_at: job.last_event_at,
        last_tool_at: job.last_tool_at,
      },
    })
  }
  if (isACPStateRunning(job.state)) {
    for (const permission of job.permissions ?? []) {
      if (!isPermissionAwaitingResponse(permission)) continue
      events.push({
        session_id: job.id,
        type: 'permission_request',
        at,
        permission,
      })
    }
  }
  return events
}

function isACPStateRunning(state?: string): boolean {
  return state === 'running' || state === 'starting'
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

// Everything the page derives from server data, computed once per data change
// (one useMemo) so renders driven by unrelated state — panel width ticks,
// composer state, live stream flags — skip the O(events) pipeline.
function deriveSessionView(data: SessionMessages, liveEvents: SessionEvent[]) {
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
    acp_active_operation: acpActiveOperation,
    acp_last_event_at: acpLastEventAt,
    acp_last_tool_at: acpLastToolAt,
    acp_children: acpChildren,
    acp_child_permissions: acpChildPermissions,
    events: persistedEvents = [],
  } = data
  // The job snapshot is only a fallback: when events record the run, the
  // snapshot would repeat it and dump every tool call at the end.
  const eventsCoverOwnACP =
    [...persistedEvents, ...liveEvents].some(
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
            model_provider: session.model_provider,
            model: session.model,
            reasoning_effort: session.reasoning_effort,
            state: acpState ?? session.status,
            assistant: eventsCoverOwnACP ? undefined : acpAssistant,
            thought: eventsCoverOwnACP ? undefined : acpThought,
            error: acpError,
            modes: acpModes,
            plan: acpPlan,
            tool_calls: eventsCoverOwnACP ? undefined : acpToolCalls,
            permissions: acpPermissions,
            active_operation: acpActiveOperation,
            last_event_at: acpLastEventAt,
            last_tool_at: acpLastToolAt,
            updated_at: session.updated_at,
          })
      : []),
  ]
  const modeEvents = [...persistedEvents, ...snapshotEvents, ...liveEvents]
  const currentModes = latestACPModeState(session.id, modeEvents) ?? acpModes
  const activePermissions = activePermissionIDs([...snapshotEvents, ...liveEvents], acpChildPermissions)
  const transcriptEvents = coalesceSessionEvents(
    [...persistedEvents, ...snapshotEvents, ...liveEvents].flatMap((event) => {
      // 'assistant' events are refresh signals; the message store has the content.
      if (event.type === 'assistant' || sessionEventPlacement(event) === 'side_chat') return []
      // Old rows round-tripped a typed-nil ACP into an empty struct.
      if (event.acp && !event.acp.id) event = { ...event, acp: undefined }
      const sanitized = sanitizeParentChildACPEvent(event, session.id)
      return sanitized ? [sanitized] : []
    }),
  )
  const settledTranscriptEvents = resolveInactivePermissions(transcriptEvents, activePermissions)
  const acpModesKnown = modeStateKnown(currentModes)
  const planAvailable = session.runtime !== 'acp' || !acpModesKnown || Boolean(currentModes?.plan_mode_id)
  const planActive = planModeActive(currentModes)
  const hasPendingPermission = activePermissions.size > 0
  const latestUserAt = Math.max(
    0,
    ...messages
      .filter((message) => message.role === 'user')
      .map((message) => Date.parse(message.created_at))
      .filter((time) => !Number.isNaN(time)),
  )
  const latestPlanDecisionEvent = settledTranscriptEvents.findLast((event) => {
    const surface = approvalPlanSurfaceFromEvent(event)
    return Boolean(
      surface?.awaitingApproval &&
        surface.approvalSessionId &&
        taskSurfaceBelongsToSession(event, session.id),
    )
  })
  const latestPlanDecisionSurface = latestPlanDecisionEvent
    ? approvalPlanSurfaceFromEvent(latestPlanDecisionEvent)
    : undefined
  const planDecisionAt = Date.parse(latestPlanDecisionEvent?.at ?? '')
  const panelProgressEvent = settledTranscriptEvents.findLast((event) =>
    Boolean(progressSurfaceFromEvent(event) && taskSurfaceBelongsToSession(event, session.id)),
  )
  // Progress lives in the side panel, never in the thread; only a proposed
  // plan that needs the user's approval stays inline. Errors are
  // notified as toasts, not rendered as rows.
  const displayEvents = coalesceSessionEvents(
    settledTranscriptEvents.flatMap((event) => {
      if (sessionEventPlacement(event) !== 'transcript') return []
      const withoutError = stripACPError(event)
      if (!hasProgressSignal(withoutError)) return [withoutError]
      return [stripProgressSignal(withoutError)]
    }),
  )
  const sideChatEvents = coalesceSessionEvents(
    [...persistedEvents, ...liveEvents].filter((event) => sessionEventPlacement(event) === 'side_chat'),
  )
  return {
    transcriptEvents: settledTranscriptEvents,
    sideChatEvents,
    displayEvents,
    planAvailable,
    planActive,
    hasPendingPermission,
    latestPlanDecisionSurface,
    planDecisionSessionID: latestPlanDecisionSurface?.approvalSessionId,
    planDecisionIsCurrent: !Number.isNaN(planDecisionAt) && planDecisionAt >= latestUserAt,
    panelProgress: panelProgressEvent ? progressSurfaceFromEvent(panelProgressEvent) : undefined,
    providerSubagents: providerSubagentsFromEvents(settledTranscriptEvents),
    spawnedThreads: spawnedThreadsFromSources(acpChildren, [...persistedEvents, ...liveEvents]),
  }
}

const SESSION_DRAFT_KEY_PREFIX = 'jaz.sessionDraft.'
const TRANSCRIPT_DOCK_GAP_PX = 20

function SessionPage({ sessionId, search }: { sessionId: string; search: SessionSearch }) {
  const queryClient = useQueryClient()
  const toast = useToast()
  const detail = useQuery(sessionMessagesQuery(sessionId))
  const events = useQuery<SessionEvent[]>({
    queryKey: keys.sessionEvents(sessionId),
    queryFn: () => [],
    initialData: [],
    staleTime: Infinity,
    gcTime: Infinity,
  })
  const streamingRef = useRef(false)
  const shownCriticalErrors = useRef(new Set<string>())
  const [lastSessionEventAt, setLastSessionEventAt] = useState<string>()
  const notifyCriticalError = useCallback((message: string) => {
    const text = message.trim()
    if (!text) return
    const toastKey = `${sessionId}:${text}`
    if (shownCriticalErrors.current.has(toastKey)) return
    shownCriticalErrors.current.add(toastKey)
    toast(text, 'danger')
  }, [sessionId, toast])
  const notifySessionEventError = useCallback((event: SessionEvent) => {
    notifyCriticalError(sessionEventErrorMessage(event))
  }, [notifyCriticalError])
  const handleSessionEvent = useCallback((event: SessionEvent) => {
    const at = event.at || new Date().toISOString()
    setLastSessionEventAt((current) => latestEventTimeISO(current, at))
    notifySessionEventError(event)
  }, [notifySessionEventError])
  useEffect(() => setLastSessionEventAt(undefined), [sessionId])
  useSessionEvents(sessionId, detail.data?.events, streamingRef, handleSessionEvent)

  const [planDecisionPending, setPlanDecisionPending] = useState(false)
  const [planDecisionError, setPlanDecisionError] = useState('')
  const sentPendingRef = useRef<string | null>(null)
  // Overview auto-opens only when it has real content, so check repo state
  // up front here; provider subagents are already in the event stream.
  const detailSession = detail.data?.session
  const sessionCwd = detailSession?.runtime_ref?.cwd
  const sideChatAvailable = isCodexACPSession(detailSession)
  const repoInfo = useQuery({ ...sessionRepoQuery(sessionId), enabled: Boolean(sessionCwd) })
  const overviewAvailable = Boolean(
    repoInfo.data?.git ||
      detail.data?.acp_children?.length ||
      detail.data?.events?.some((event) => sessionEventPlacement(event) === 'overview') ||
      events.data.some((event) => sessionEventPlacement(event) === 'overview'),
  )
  const sidePanel = useSidePanelState(overviewAvailable, sideChatAvailable)
  const { openFile } = sidePanel
  // Phone: the docked panel would crush the transcript to a sliver, so it
  // becomes a full-screen overlay (CSS `max-sm:w-full`) that slides in instead
  // of a column.
  const isMobile = useIsMobile()

  const itemCount =
    (detail.data?.messages.length ?? 0) + events.data.filter((event) => sessionEventPlacement(event) !== 'side_chat').length
  const { live, streaming, send: sendLiveMessage, abort: abortLiveMessage } = useLiveSessionSend({
    sessionId,
    streamingRef,
    onCriticalError: notifyCriticalError,
  })
  const liveSize = liveExchangeSize(live)
  const [bottomDockHeight, setBottomDockHeight] = useState(0)
  const transcriptBottomPadding = Math.max(bottomDockHeight + TRANSCRIPT_DOCK_GAP_PX, 160)
  const { scrollRef, showScrollToBottom, onScroll: onThreadScroll, scrollToBottom, pinToBottom } =
    useThreadAutoScroll({ resetKey: sessionId, itemCount, liveSize, bottomInset: transcriptBottomPadding })
  const threadFind = useThreadFind(sessionId, scrollRef)
  const [highlightedMessageSeq, setHighlightedMessageSeq] = useState<number>()

  const handleSend = useCallback((text: string, options: SendMessageOptions = {}) => {
    pinToBottom()
    sendLiveMessage(text, options)
  }, [pinToBottom, sendLiveMessage])

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
    queryClient.invalidateQueries({ queryKey: keys.usage })
  }, [handleSend, queryClient, sessionId])

  const handleSideChatSend = useCallback(async (
    sideChatID: string,
    message: string,
    options: SendMessageOptions = {},
  ) => {
    const uploaded = options.files?.length
      ? await Promise.all(options.files.map((file) => uploadSessionAttachment(sessionId, file)))
      : []
    const prepared = preparedSendMessage(options, uploaded)
    await sendSessionSideChat(sessionId, {
      id: sideChatID,
      message,
      contexts: prepared.contexts,
      attachment_ids: prepared.attachmentIds,
    })
    await queryClient.refetchQueries({ queryKey: keys.sessionMessages(sessionId) })
  }, [queryClient, sessionId])

  const currentSession = detail.data?.session
  const queue = useSessionQueue({
    sessionId,
    session: currentSession,
    acpState: detail.data?.acp_state,
    streaming,
    onSend: handleSend,
  })
  const composerContexts = useComposerContexts({
    storageKey: `${SESSION_DRAFT_KEY_PREFIX}${sessionId}`,
    storage: 'local',
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

  useEffect(() => {
    if (!detail.isSuccess || !search.message) return
    const messageSeq = search.message
    setHighlightedMessageSeq(messageSeq)
    const frame = requestAnimationFrame(() => {
      const target = scrollRef.current?.querySelector<HTMLElement>(`[data-message-seq="${messageSeq}"]`)
      target?.scrollIntoView({ block: 'center', inline: 'nearest' })
    })
    const timer = window.setTimeout(() => {
      setHighlightedMessageSeq((current) => (current === messageSeq ? undefined : current))
    }, 2200)
    return () => {
      cancelAnimationFrame(frame)
      window.clearTimeout(timer)
    }
  }, [detail.isSuccess, detail.data?.messages.length, scrollRef, search.message, sessionId])

  const data = detail.data
  const derived = useMemo(
    () => (data ? deriveSessionView(data, events.data) : undefined),
    [data, events.data],
  )
  if (detail.isPending) {
    return (
      <div className="mx-auto max-w-[var(--thread-max)] px-10">
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

  if (!derived) return null // unreachable: derived exists whenever detail.data does
  const { session, messages } = detail.data
  const {
    transcriptEvents,
    displayEvents,
    planAvailable,
    planActive,
    hasPendingPermission,
    latestPlanDecisionSurface,
    planDecisionSessionID,
    planDecisionIsCurrent,
    panelProgress,
    providerSubagents,
    spawnedThreads,
    sideChatEvents,
  } = derived
  const showPlanDecision = Boolean(
    latestPlanDecisionSurface?.awaitingApproval &&
      planDecisionSessionID &&
      planDecisionIsCurrent &&
      !streaming &&
      !live &&
      !hasPendingPermission,
  )
  const sessionError = session.status === 'error' ? session.error?.trim() || 'Unknown error.' : ''
  const sessionErrorContext = [session.model_provider, session.model].filter(Boolean).join(' · ')
  const isACP = session.runtime === 'acp'
  // Covers turns started elsewhere (parent-triggered, or refresh mid-turn).
  const sessionRunning = queue.sessionRunning
  const pendingSteer = session.pending_steer_message
  const empty = messages.length === 0 && transcriptEvents.length === 0 && !live && !sessionError && !sessionRunning
  // ACP turns stream through events; the live exchange only contributes the
  // not-yet-refetched user bubble, injected so mid-turn events sort after it.
  const lastUserMessage = messages.findLast((message) => message.role === 'user')
  const transcriptMessages =
    isACP && live && lastUserMessage?.content.trim() !== live.user.trim()
      ? [
          ...messages,
          liveUserMessage(live, (messages.at(-1)?.seq ?? 0) + 1_000_000),
        ]
      : messages

  return (
    <FileReaderLinkProvider onOpen={openFile}>
      <PreviewLinkProvider onOpen={sidePanel.openPreview}>
        {/* Phone: the closed side panel slides off to the right (translateX 100%);
            clip horizontal overflow so it can't be revealed by scrolling. */}
        <FileDropScope ref={sidePanel.measureRef} className="relative flex h-full max-sm:overflow-x-clip">
          <SessionTitlebar
            session={session}
            isMobile={isMobile}
            sidePanelOpen={sidePanel.open}
            sidePanelView={sidePanel.view}
            sideChatAvailable={sideChatAvailable}
            fileAvailable={Boolean(sidePanel.fileRef)}
            onToggleSidePanel={sidePanel.toggle}
            onSelectSidePanelView={sidePanel.selectView}
          />
          {/* Phone: the open panel covers the chat full-width, so the only
              non-panel area left is the title bar. This catches taps on its empty
              space (the header controls sit above it) to dismiss the panel. */}
          {isMobile && sidePanel.open ? (
            <div className="fixed inset-0 z-scrim" aria-hidden onClick={() => sidePanel.toggle()} />
          ) : null}

          <div className="relative h-full min-w-0 flex-1">
            <div ref={scrollRef} className="h-full overflow-y-auto" onScroll={onThreadScroll}>
              <div
                ref={threadFind.rootRef}
                className={`${THREAD_COLUMN_CLASS} pt-2`}
                style={{ paddingBottom: transcriptBottomPadding }}
              >
                {empty ? (
                  <EmptyState title="Start the conversation">
                    <p>Messages stream in live as your assistant thinks and works.</p>
                  </EmptyState>
                ) : (
                  <>
                    <Transcript
                      key={session.id}
                      messages={transcriptMessages}
                      events={displayEvents}
                      sessionId={session.id}
                      groupTurns={isACP}
                      working={sessionRunning}
                      findActive={threadFind.open && Boolean(threadFind.query.trim())}
                      highlightedSeq={highlightedMessageSeq}
                      onArtifactPrompt={queue.onSend}
                      tail={
                        isACP ? (
                          <>
                            {pendingSteer ? <PendingSteerBubble prompt={pendingSteer} /> : null}
                            <SessionLivenessIndicator
                              agent={session.runtime_ref?.agent}
                              running={sessionRunning}
                              activeOperation={detail.data?.acp_active_operation}
                              updatedAt={session.updated_at}
                              lastActivityAt={latestEventTimeISO(lastSessionEventAt, live?.at)}
                            />
                          </>
                        ) : live ? (
                          <div className="flex flex-col gap-5">
                            <motion.div
                              className="flex justify-end"
                              initial={{ opacity: 0, y: 8 }}
                              animate={{ opacity: 1, y: 0 }}
                              transition={{ type: 'spring', stiffness: 380, damping: 30 }}
                            >
                              <div className="min-w-0 max-w-[84%] rounded-card bg-surface px-3.5 py-2.5 text-sm whitespace-pre-wrap [overflow-wrap:break-word] select-text">
                                <MessageContexts contexts={live.contexts} />
                                <MentionText text={live.user} />
                                <LiveAttachmentList attachments={live.attachments} />
                              </div>
                            </motion.div>
                            <ThinkingBlock text={live.reasoning} pending={streaming} />
                            {live.tools.map((tool) =>
                              isArtifactToolName(tool.name) ? (
                                <ArtifactBlock
                                  key={tool.key}
                                  args={tool.args}
                                  result={tool.result}
                                  pending={streaming && tool.result === undefined}
                                  onSendPrompt={queue.onSend}
                                />
                              ) : (
                                <ToolCallCard
                                  key={tool.key}
                                  name={tool.name}
                                  args={tool.args}
                                  result={tool.result}
                                  pending={streaming && tool.result === undefined}
                                />
                              ),
                            )}
                            {live.assistant ? (
                              <MessageMarkdown text={live.assistant} />
                            ) : streaming ? (
                              <p className="animate-pulse text-sm text-ink-3">Thinking…</p>
                            ) : null}
                            {live.error ? <SessionErrorNotice message={live.error} /> : null}
                          </div>
                        ) : null
                      }
                    />
                    {sessionError ? (
                      <SessionErrorNotice message={sessionError} context={sessionErrorContext} className="mt-5" />
                    ) : null}
                  </>
                )}
              </div>
            </div>
            <ThreadFindBar find={threadFind} />
            <SelectionContextToolbar scrollRef={scrollRef} onAdd={composerContexts.addSelection} />

            {showPlanDecision && planDecisionError ? (
              <p className="absolute inset-x-0 bottom-32 mx-auto max-w-[640px] rounded-card bg-danger-soft px-3 py-2 text-sm text-danger select-text">
                {planDecisionError}
              </p>
            ) : null}
            <BottomDock
              before={<ScrollToBottomButton visible={showScrollToBottom} onClick={scrollToBottom} />}
              onHeightChange={setBottomDockHeight}
            >
              {showPlanDecision ? (
                <PlanDecisionCard
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
              ) : (
                <Composer
                  streaming={sessionRunning}
                  planAvailable={planAvailable}
                  planModeActive={Boolean(live?.planRequested) || planActive}
                  queuedPrompts={queue.queuedPrompts}
                  steerDisabled={queue.steerDisabled}
                  draftStorageKey={`${SESSION_DRAFT_KEY_PREFIX}${session.id}`}
                  fileRoot={session.runtime_ref?.cwd}
                  contexts={composerContexts.contexts}
                  onRemoveContext={composerContexts.removeContext}
                  onClearContexts={composerContexts.clearContexts}
                  onSend={queue.onSend}
                  onStop={() => {
                    // the turn runs detached server-side; stop it there first
                    void cancelSession(sessionId).catch(() => {})
                    abortLiveMessage()
                  }}
                  onVoice={undefined}
                  onUploadAttachment={(file) => uploadSessionAttachment(session.id, file)}
                  onSteerQueuedPrompt={queue.onSteerQueuedPrompt}
                  onDeleteQueuedPrompt={queue.onDeleteQueuedPrompt}
                  onEditQueuedPrompt={queue.onEditQueuedPrompt}
                  onReorderQueuedPrompts={queue.onReorderQueuedPrompts}
                />
              )}
            </BottomDock>
          </div>

          {/* Docked, never overlapping: the chat pane flexes and stays centered
              between the sidebar and this panel. */}
          <motion.div
            className="h-full shrink-0 overflow-hidden max-sm:absolute max-sm:inset-y-0 max-sm:right-0 max-sm:z-shell max-sm:w-full!"
            initial={false}
            // The fixed backdrop above owns tap-to-dismiss.
            animate={drawerSlide({ isMobile, open: sidePanel.open, side: 'right', width: sidePanel.width })}
            transition={{ type: 'spring', stiffness: 400, damping: 36 }}
          >
            <SidePanel
              session={session}
              progress={panelProgress}
              subagents={providerSubagents}
              spawnedThreads={spawnedThreads}
              working={sessionRunning}
              visible={sidePanel.open}
              view={sidePanel.view}
              previewUrl={sidePanel.previewUrl}
              fileRef={sidePanel.fileRef}
              sideChatAvailable={sideChatAvailable}
              sideChatEvents={sideChatEvents}
              onPreviewUrlChange={sidePanel.setPreviewUrl}
              onOpenFile={openFile}
              onAddBrowserAnnotation={composerContexts.addBrowserAnnotation}
              onUploadAttachment={(file) => uploadSessionAttachment(session.id, file)}
              onSend={queue.onSend}
              onSendSideChat={handleSideChatSend}
              onClose={sidePanel.toggle}
            />
          </motion.div>
        </FileDropScope>
      </PreviewLinkProvider>
    </FileReaderLinkProvider>
  )
}
