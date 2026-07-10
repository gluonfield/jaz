import { useQuery, useQueryClient } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { ArrowDown, Play } from 'lucide-react'
import { AnimatePresence, motion } from 'motion/react'
import { useCallback, useEffect, useMemo, useRef, useState, type CSSProperties } from 'react'
import { BottomDock } from '@/components/session/BottomDock'
import { Composer, PlanDecisionCard } from '@/components/session/Composer'
import { LiveAttachmentList } from '@/components/session/LiveAttachmentList'
import { MessageContexts } from '@/components/session/MessageContexts'
import { SelectionContextToolbar } from '@/components/session/SelectionContextToolbar'
import { useComposerContexts } from '@/components/session/useComposerContexts'
import { FileReaderLinkProvider, MessageMarkdown, PreviewLinkProvider } from '@/components/session/MessageMarkdown'
import { MentionText } from '@/components/session/mentions'
import { SessionErrorNotice, type SessionErrorAction } from '@/components/session/SessionErrorNotice'
import { SessionLivenessIndicator } from '@/components/session/SessionLivenessIndicator'
import { GoalStatusBar } from '@/components/session/GoalStatusBar'
import { PendingSteerBubble } from '@/components/session/PendingSteerBubble'
import { SidePanel, type SidePanelView } from '@/components/session/SidePanel'
import { SidePanelResizeHandle } from '@/components/session/SidePanelResizeHandle'
import { SidePanelControl, useSidePanelState } from '@/components/session/SidePanelState'
import { RuntimeBadge } from '@/components/sidebar/RuntimeBadge'
import { ArtifactBlock } from '@/components/session/ArtifactBlock'
import { ThinkingBlock } from '@/components/session/ThinkingBlock'
import { ThreadFindBar } from '@/components/session/ThreadFindBar'
import { TokenStats } from '@/components/session/TokenStats'
import { ToolCallCard } from '@/components/session/ToolCallCard'
import { Transcript } from '@/components/session/Transcript'
import { deriveSessionView, isCodexACPSession, sessionEventErrorMessage } from '@/components/session/sessionView'
import { THREAD_COLUMN_CLASS } from '@/components/session/threadLayout'
import { isArtifactToolName } from '@/components/session/toolVisibility'
import { useThreadFind } from '@/components/session/useThreadFind'
import { useThreadAutoScroll } from '@/components/session/useThreadAutoScroll'
import { liveTranscriptMessages, useLiveSessionSend } from '@/components/session/useLiveSessionSend'
import { EmptyState } from '@/components/ui/EmptyState'
import { FileDropScope } from '@/components/ui/FileDrop'
import { Skeleton, SkeletonRows } from '@/components/ui/Skeleton'
import { useToast } from '@/components/ui/toast'
import { markThreadSeen } from '@/lib/api/feed'
import {
  answerSessionInteractiveResponse,
  cancelSession,
  sendSessionSideChat,
  sessionMessagesQuery,
  uploadSessionAttachment,
} from '@/lib/api/sessions'
import type { Session, SessionEvent } from '@/lib/api/types'
import { drawerSlide } from '@/lib/dom/drawer'
import { useIsMobile } from '@/lib/hooks/useIsMobile'
import { useSessionEvents } from '@/lib/hooks/useSessionEvents'
import { useSessionQueue } from '@/lib/hooks/useSessionQueue'
import { takePendingMessage } from '@/lib/pendingMessage'
import { keys } from '@/lib/query/keys'
import { type PlanApprovalAction } from '@/lib/taskSurface'
import { preparedSendMessage, type SendMessageOptions } from '@/lib/sendMessage'
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
  const detailSession = detail.data?.session
  const sideChatAvailable = isCodexACPSession(detailSession)
  const sidePanel = useSidePanelState(sideChatAvailable)
  // Phone: the docked panel would crush the transcript to a sliver, so it
  // becomes a full-screen overlay (CSS `max-sm:w-full`) that slides in instead
  // of a column.
  const isMobile = useIsMobile()

  const { live, streaming, send: sendLiveMessage, abort: abortLiveMessage } = useLiveSessionSend({
    sessionId,
    streamingRef,
    onCriticalError: notifyCriticalError,
  })
  const [bottomDockHeight, setBottomDockHeight] = useState(0)
  const transcriptBottomPadding = Math.max(bottomDockHeight + TRANSCRIPT_DOCK_GAP_PX, 160)
  const { scrollRef, attachScroll, showScrollToBottom, onScroll: onThreadScroll, scrollToBottom, pinToBottom } =
    useThreadAutoScroll({ resetKey: sessionId })
  const threadFind = useThreadFind(sessionId, scrollRef)
  const [highlightedMessageSeq, setHighlightedMessageSeq] = useState<number>()

  const handleSend = useCallback((text: string, options: SendMessageOptions = {}) => {
    pinToBottom()
    sendLiveMessage(text, options)
  }, [pinToBottom, sendLiveMessage])

  // Cancelling clears any active goal server-side, so this doubles as the goal
  // off-switch: it stops a running turn and the auto-continuation loop.
  const stopSession = useCallback(() => {
    // The turn runs detached server-side; clear local optimistic state now.
    abortLiveMessage()
    void cancelSession(sessionId)
      .catch(() => {})
      .finally(() => {
        queryClient.invalidateQueries({ queryKey: keys.sessionMessages(sessionId) })
        queryClient.invalidateQueries({ queryKey: keys.sidebarSessions })
        queryClient.invalidateQueries({ queryKey: keys.allSessions })
        queryClient.invalidateQueries({ queryKey: keys.usage })
      })
  }, [abortLiveMessage, queryClient, sessionId])

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

  const answerPlanApproval = useCallback(async (
    approval: PlanApprovalAction,
    action: 'implement' | 'clarify',
    text = '',
  ) => {
    const parentVisible = approval.sessionId !== sessionId
    if (approval.type === 'message') {
      await sendACPFallback(
        approval.sessionId,
        action === 'implement' ? 'Implement the plan.' : text,
        {
          planRequested: action === 'clarify',
          parentVisible,
        },
      )
      return
    }

    if (action === 'implement') {
      await answerSessionInteractiveResponse(approval.sessionId, {
        request_id: approval.requestId,
        option_id: approval.approveOptionId,
        parent_visible: parentVisible,
      })
    } else {
      await answerSessionInteractiveResponse(approval.sessionId, {
        request_id: approval.requestId,
        option_id: approval.clarifyOptionId,
        text,
        plan_requested: true,
        parent_visible: parentVisible,
      })
    }
    queryClient.invalidateQueries({ queryKey: keys.sessionMessages(approval.sessionId) })
    queryClient.invalidateQueries({ queryKey: keys.sidebarSessions })
    queryClient.invalidateQueries({ queryKey: keys.allSessions })
    queryClient.invalidateQueries({ queryKey: keys.usage })
  }, [queryClient, sendACPFallback, sessionId])

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
    handleSend(pending.text, {
      planRequested: pending.planRequested,
      goalRequested: pending.goalRequested,
      files: pending.files ?? [],
    })
  }, [detail.isSuccess, handleSend, sessionId])

  useEffect(() => {
    void markThreadSeen(sessionId).finally(() => queryClient.invalidateQueries({ queryKey: keys.feed }))
  }, [sessionId, queryClient])

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
    goalAvailable,
    goalActive,
    goalRequested,
    goal,
    hasBlockingPendingPermission,
    latestPlanDecisionSurface,
    planDecisionApproval,
    planDecisionIsCurrent,
    panelProgress,
    providerSubagents,
    spawnedThreads,
    sideChatEvents,
  } = derived
  const showPlanDecision = Boolean(
    latestPlanDecisionSurface?.awaitingApproval &&
      planDecisionApproval &&
      planDecisionIsCurrent &&
      !streaming &&
      !live &&
      !hasBlockingPendingPermission,
  )
  const sessionError = session.status === 'error' ? session.error?.trim() || 'Unknown error.' : ''
  const sessionErrorContext = [session.model_provider, session.model].filter(Boolean).join(' · ')
  const isACP = session.runtime === 'acp'
  // Covers turns started elsewhere (parent-triggered, or refresh mid-turn).
  const sessionRunning = queue.sessionRunning
  const pendingSteer = session.pending_steer_message
  const empty = messages.length === 0 && transcriptEvents.length === 0 && !live && !sessionError && !sessionRunning
  // ACP turns stream through events. While the request is active, the local
  // send time is the turn boundary; replayed user rows can be timestamped after
  // early live events and would otherwise fold those events into the prior turn.
  const transcriptMessages = liveTranscriptMessages(messages, live, isACP)
  const goalStatusVisible = goalActive
  const canContinueFromInlineError =
    isACP && !sessionRunning && !streaming && !live && !hasBlockingPendingPermission
  const continueErrorAction: SessionErrorAction | undefined =
    canContinueFromInlineError ? {
      label: 'Continue',
      icon: <Play size={13} aria-hidden />,
      onClick: () => handleSend('Continue'),
      title: 'Continue this thread',
    } : undefined

  return (
    <FileReaderLinkProvider onOpen={sidePanel.openFile}>
      <PreviewLinkProvider onOpen={sidePanel.openPreview}>
        {/* Phone: the closed side panel slides off to the right (translateX 100%);
            clip horizontal overflow so it can't be revealed by scrolling. */}
        <FileDropScope className="relative flex h-full max-sm:overflow-x-clip">
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
          {sidePanel.resizing ? <div className="fixed inset-0 z-modal cursor-col-resize" aria-hidden /> : null}

          <div className="relative h-full min-w-0 flex-1">
            <div ref={attachScroll} className="scrollbar-quiet h-full overflow-y-auto" onScroll={onThreadScroll}>
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
                      errorAction={sessionError ? undefined : continueErrorAction}
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
                                <LiveAttachmentList attachments={live.attachments} attachmentSessionId={sessionId} />
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
                      <SessionErrorNotice
                        message={sessionError}
                        context={sessionErrorContext}
                        className="mt-5"
                        action={continueErrorAction}
                      />
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
                    if (!planDecisionApproval) return
                    setPlanDecisionPending(true)
                    setPlanDecisionError('')
                    void answerPlanApproval(planDecisionApproval, 'implement')
                      .catch((err: Error) => setPlanDecisionError(err.message || 'Sending the approval failed.'))
                      .finally(() => setPlanDecisionPending(false))
                  }}
                  onClarify={(text) => {
                    if (!planDecisionApproval) return
                    setPlanDecisionPending(true)
                    setPlanDecisionError('')
                    void answerPlanApproval(planDecisionApproval, 'clarify', text)
                      .catch((err: Error) => setPlanDecisionError(err.message || 'Sending the reply failed.'))
                      .finally(() => setPlanDecisionPending(false))
                  }}
                />
              ) : (
                <>
                  {goalStatusVisible ? <GoalStatusBar goal={goal} /> : null}
                  <Composer
                    streaming={sessionRunning}
                    planAvailable={planAvailable}
                    planModeActive={Boolean(live?.planRequested) || planActive}
                    goalControlVisible
                    goalAvailable={goalAvailable}
                    goalEngaged={goalRequested || goalActive}
                    queuedPrompts={queue.queuedPrompts}
                    steerDisabled={queue.steerDisabled}
                    draftStorageKey={`${SESSION_DRAFT_KEY_PREFIX}${session.id}`}
                    fileRoot={session.runtime_ref?.cwd}
                    contexts={composerContexts.contexts}
                    onRemoveContext={composerContexts.removeContext}
                    onClearContexts={composerContexts.clearContexts}
                    onSend={queue.onSend}
                    onStop={stopSession}
                    onClearGoal={stopSession}
                    onVoice={undefined}
                    onUploadAttachment={(file) => uploadSessionAttachment(session.id, file)}
                    onSteerQueuedPrompt={queue.onSteerQueuedPrompt}
                    onDeleteQueuedPrompt={queue.onDeleteQueuedPrompt}
                    onEditQueuedPrompt={queue.onEditQueuedPrompt}
                    onReorderQueuedPrompts={queue.onReorderQueuedPrompts}
                  />
                </>
              )}
            </BottomDock>
          </div>

          {/* Docked, never overlapping: the chat pane flexes and stays centered
              between the sidebar and this panel. */}
          <motion.div
            style={{ '--side-panel-width': `${sidePanel.width}px` } as CSSProperties}
            className="relative h-full shrink-0 overflow-hidden max-sm:absolute max-sm:inset-y-0 max-sm:right-0 max-sm:z-shell max-sm:w-full!"
            initial={false}
            // The fixed backdrop above owns tap-to-dismiss.
            animate={drawerSlide({ isMobile, open: sidePanel.open, side: 'right', width: sidePanel.width })}
            transition={sidePanel.resizing ? { duration: 0 } : { type: 'spring', stiffness: 400, damping: 36 }}
          >
            {sidePanel.resizable ? (
              <SidePanelResizeHandle
                width={sidePanel.width}
                minWidth={sidePanel.defaultWidth}
                maxWidth={sidePanel.maxWidth}
                disabled={isMobile || !sidePanel.open}
                onResizeStart={() => sidePanel.setResizing(true)}
                onResize={sidePanel.resize}
                onResizeEnd={() => sidePanel.setResizing(false)}
              />
            ) : null}
            <SidePanel
              session={session}
              progress={panelProgress}
              subagents={providerSubagents}
              spawnedThreads={spawnedThreads}
              working={sessionRunning}
              visible={sidePanel.open}
              view={sidePanel.view}
              previewTarget={sidePanel.previewTarget}
              fileRef={sidePanel.fileRef}
              sideChatAvailable={sideChatAvailable}
              sideChatEvents={sideChatEvents}
              onPreviewTargetChange={sidePanel.setPreviewTarget}
              onOpenFile={sidePanel.openFile}
              onAddBrowserAnnotation={composerContexts.addBrowserAnnotation}
              onUploadAttachment={(file) => uploadSessionAttachment(session.id, file)}
              onSend={queue.onSend}
              onQueuePrompt={queue.onQueuePrompt}
              onQueueAction={queue.onQueueAction}
              onSendSideChat={handleSideChatSend}
              onClose={sidePanel.toggle}
            />
          </motion.div>
        </FileDropScope>
      </PreviewLinkProvider>
    </FileReaderLinkProvider>
  )
}
