import { useComputerControl } from '@/lib/hooks/useComputerControl'
import { usePrefetchQuery, useQuery, useQueryClient } from '@tanstack/react-query'
import { useLocation, useNavigate } from '@tanstack/react-router'
import { ArrowDown, Play } from 'lucide-react'
import { AnimatePresence, motion } from 'motion/react'
import { type ReactNode, type RefObject, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { BottomDock } from '@/components/session/BottomDock'
import { UserBubble } from '@/components/session/Bubble'
import { Composer, PlanDecisionCard } from '@/components/session/Composer'
import { NativeModelOptions } from '@/components/session/NativeModelOptions'
import { useSessionConfig } from '@/components/session/useSessionConfig'
import { SelectionContextToolbar } from '@/components/session/SelectionContextToolbar'
import { useComposerContexts } from '@/components/session/useComposerContexts'
import { FileReaderLinkProvider, MessageMarkdown, PreviewLinkProvider } from '@/components/session/MessageMarkdown'
import { SessionErrorNotice, type SessionErrorAction } from '@/components/session/SessionErrorNotice'
import { SessionLivenessIndicator } from '@/components/session/SessionLivenessIndicator'
import { GoalStatusBar } from '@/components/session/GoalStatusBar'
import { PendingSteerBubble } from '@/components/session/PendingSteerBubble'
import { PendingSessionHistory } from '@/components/session/PendingSessionHistory'
import { SidePanel } from '@/components/session/SidePanel'
import { SessionTitlebar } from '@/components/session/SessionTitlebar'
import { SidePanelDrawer } from '@/components/session/SidePanelDrawer'
import { useSidePanelState } from '@/components/session/SidePanelState'
import { ThinkingBlock } from '@/components/session/ThinkingBlock'
import { ThreadFindBar } from '@/components/session/ThreadFindBar'
import { ThreadOutline } from '@/components/session/ThreadOutline'
import { ToolCalls } from '@/components/session/ToolCalls'
import { Transcript } from '@/components/session/Transcript'
import { deriveSessionView, isCodexACPSession, sessionEventErrorMessage } from '@/components/session/sessionView'
import { THREAD_COLUMN_CLASS } from '@/components/session/threadLayout'
import { useThreadFind } from '@/components/session/useThreadFind'
import { useThreadAutoScroll } from '@/components/session/useThreadAutoScroll'
import { liveOptimisticUserMessage } from '@/components/session/liveTranscript'
import { useLiveSessionSend } from '@/components/session/useLiveSessionSend'
import { EmptyState } from '@/components/ui/EmptyState'
import { Button } from '@/components/ui/Button'
import { FileDropScope } from '@/components/ui/FileDrop'
import { useToast } from '@/components/ui/toast'
import { markThreadSeen } from '@/lib/api/feed'
import { voiceAvailableQuery } from '@/lib/api/liveVoice'
import {
  answerSessionInteractiveResponse,
  cancelSession,
  clearSessionGoal,
  sendSessionSideChat,
  sessionEventsQuery,
  sessionOverviewQuery,
  sessionRepoQuery,
  uploadSessionAttachment,
} from '@/lib/api/sessions'
import type { AgentSessionState, ChatMessage, SessionEvent, SessionOverview } from '@/lib/api/types'
import { useIsMobile } from '@/lib/hooks/useIsMobile'
import { useSessionEvents } from '@/lib/hooks/useSessionEvents'
import { type HistoryPaging, useSessionHistory } from '@/lib/hooks/useSessionHistory'
import { useSessionQueue } from '@/lib/hooks/useSessionQueue'
import { useVoiceMode } from '@/lib/hooks/useVoiceMode'
import { VoiceMode } from '@/components/session/VoiceMode'
import { invalidateSessionLists } from '@/lib/query/invalidate'
import { keys } from '@/lib/query/keys'
import { type PlanApprovalAction } from '@/lib/taskSurface'
import { preparedSendMessage, type SendMessageOptions } from '@/lib/sendMessage'
import { latestEventTimeISO } from '@/lib/sessionLiveness'
import type { ProviderSubagentView } from '@/lib/providerSubagents'
import type { SpawnedThreadView } from '@/lib/spawnedThreads'
import {
  type OptimisticUserMessage,
  optimisticTranscriptMessages,
  pendingOptimisticUserMessage,
} from '@/lib/optimisticUserMessage'

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
          // It floats over live transcript content, including surfaces it shares
          // a fill with, so it needs an edge of its own to read as a control
          // rather than a hole punched in whatever is behind it.
          className="mx-auto mb-2 grid size-10 place-items-center rounded-full bg-surface text-ink shadow-[0_8px_24px_rgba(0,0,0,0.14)] ring-1 ring-border transition-colors duration-150 hover:bg-surface-2"
        >
          <ArrowDown size={17} />
        </motion.button>
      ) : null}
    </AnimatePresence>
  )
}

const SESSION_DRAFT_KEY_PREFIX = 'jaz.sessionDraft.'
const TRANSCRIPT_DOCK_GAP_PX = 20
const EMPTY_OVERVIEW: SessionOverview = { threads: [], subagents: [] }

export interface ThreadChatView {
  messages: ChatMessage[]
  events: SessionEvent[]
  scrollRef: RefObject<HTMLDivElement | null>
  paging: HistoryPaging
  working: boolean
  threads: SpawnedThreadView[]
  send: (text: string) => void
  showDetails: () => void
}

export interface ThreadDetailsView {
  agentSession?: AgentSessionState
  working: boolean
  threads: SpawnedThreadView[]
  subagents: ProviderSubagentView[]
}

// One thread's full view: transcript, composer, and side panel. `header` and
// `details` let an owning surface (a bot) replace the titlebar identity and the
// Overview panel, and `openDetails` decide whether that panel starts open;
// `chat` replaces the agent transcript with a chat log and the composer with
// its messenger layout.
export function ThreadView({
  sessionId,
  message,
  initialPrompt,
  header,
  details,
  openDetails,
  placeholder,
  chat,
}: {
  sessionId: string
  message?: number
  initialPrompt?: OptimisticUserMessage
  header?: ReactNode
  details?: (view: ThreadDetailsView) => ReactNode
  openDetails?: boolean
  placeholder?: string
  chat?: (view: ThreadChatView) => ReactNode
}) {
  const queryClient = useQueryClient()
  const config = useSessionConfig(sessionId)
  const toast = useToast()
  const reportHistoryError = useCallback((message: string) => {
    toast(`Couldn't load earlier history: ${message}`, 'danger')
  }, [toast])
  useComputerControl(sessionId)
  const detail = useSessionHistory(sessionId, reportHistoryError)
  const voice = useVoiceMode(sessionId)
  const voiceAvailable = useQuery(voiceAvailableQuery).data
  const { start: startVoiceConversation } = voice
  const startVoice = useLocation({ select: (location) => location.state.startVoice })
  const navigate = useNavigate()
  useEffect(() => {
    if (!startVoice) return
    void navigate({ replace: true, state: (previous) => ({ ...previous, startVoice: undefined }) })
    startVoiceConversation()
  }, [navigate, startVoice, startVoiceConversation])
  const overview = useQuery(sessionOverviewQuery(sessionId))
  usePrefetchQuery(sessionRepoQuery(sessionId))
  const events = useQuery(sessionEventsQuery(sessionId))
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
  useSessionEvents(sessionId, detail.data?.latest_event_seq, handleSessionEvent)

  const { paging } = detail

  const [planDecisionPending, setPlanDecisionPending] = useState(false)
  const [planDecisionError, setPlanDecisionError] = useState('')
  const seenRequestedRef = useRef(false)
  const detailSession = detail.data?.session
  const sideChatAvailable = isCodexACPSession(detailSession)
  const sidePanel = useSidePanelState(sessionId, sideChatAvailable, openDetails)
  const { open: panelOpen, mode: panelMode, toggleMode: togglePanel } = sidePanel
  const showDetails = useCallback(() => {
    if (!panelOpen || panelMode !== 'overview') togglePanel('overview')
  }, [panelOpen, panelMode, togglePanel])
  // Phone: the docked panel would crush the transcript to a sliver, so it
  // becomes a full-screen overlay (CSS `max-sm:w-full`) that slides in instead
  // of a column.
  const isMobile = useIsMobile()

  const { live, streaming, send: sendLiveMessage, abort: abortLiveMessage } = useLiveSessionSend({
    sessionId,
    onCriticalError: notifyCriticalError,
  })
  const [bottomDockHeight, setBottomDockHeight] = useState(0)
  const transcriptBottomPadding = Math.max(bottomDockHeight + TRANSCRIPT_DOCK_GAP_PX, 160)
  const {
    scrollRef,
    attachScroll,
    showScrollToBottom,
    onScroll: onThreadScroll,
    onClickCapture: onThreadClickCapture,
    scrollToBottom,
    pinToBottom,
    pauseFollowing,
  } = useThreadAutoScroll({ resetKey: sessionId })
  const currentSession = detail.data?.session
  const queue = useSessionQueue({
    sessionId,
    session: currentSession,
    acpState: detail.data?.acp_state,
    streaming,
    onSend: sendLiveMessage,
  })
  const sendMessage = queue.onSend
  const handleSend = useCallback((text: string, options: SendMessageOptions = {}) => {
    pinToBottom()
    return sendMessage(text, options)
  }, [pinToBottom, sendMessage])
  const threadFind = useThreadFind(sessionId, scrollRef, pauseFollowing)
  const [revealedMessageSeq, setRevealedMessageSeq] = useState<number>()
  const jumpedMessageRef = useRef(0)

  // The one way into a message: deep links and the outline rail both land here.
  // A jump only reveals and scrolls — it never marks or focuses the target.
  const jumpToMessage = useCallback((messageSeq: number) => {
    pauseFollowing()
    setRevealedMessageSeq(messageSeq)
    requestAnimationFrame(() => {
      scrollRef.current
        ?.querySelector<HTMLElement>(`[data-message-seq="${messageSeq}"]`)
        ?.scrollIntoView({ block: 'center', inline: 'nearest' })
    })
  }, [pauseFollowing, scrollRef])

  const stopSession = useCallback(() => {
    // The turn runs detached server-side; clear local optimistic state now.
    abortLiveMessage()
    void cancelSession(sessionId)
      .catch(() => {})
      .finally(() => {
        queryClient.invalidateQueries({ queryKey: keys.sessionMessages(sessionId) })
        queryClient.invalidateQueries({ queryKey: keys.sidebarSessions })
        queryClient.invalidateQueries({ queryKey: keys.usage })
      })
  }, [abortLiveMessage, queryClient, sessionId])

  const clearGoal = useCallback(() => {
    void clearSessionGoal(sessionId)
      .then(() => queryClient.invalidateQueries({ queryKey: keys.sessionMessages(sessionId) }))
      .catch((error) => toast(error instanceof Error ? error.message : 'Could not turn Goal mode off', 'danger'))
  }, [queryClient, sessionId, toast])

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

  const composerContexts = useComposerContexts({
    storageKey: `${SESSION_DRAFT_KEY_PREFIX}${sessionId}`,
    storage: 'local',
  })

  useEffect(() => {
    if (seenRequestedRef.current) return
    seenRequestedRef.current = true
    void markThreadSeen(sessionId).finally(() => invalidateSessionLists(queryClient))
  }, [sessionId, queryClient])

  // A deep link jumps once. Without this the effect re-fires on every streamed
  // update and drags the reader back to the linked message after they have
  // navigated away from it.
  useEffect(() => {
    if (!detail.isSuccess || !message || jumpedMessageRef.current === message) return
    if (!detail.data.messages.some((item) => item.seq === message)) {
      if (paging.hasEarlier && !paging.loading) void paging.loadEarlier()
      return
    }
    jumpedMessageRef.current = message
    jumpToMessage(message)
  }, [detail.data, detail.isSuccess, jumpToMessage, paging, message])

  const data = detail.data
  const overviewData = overview.data ?? (overview.isError ? undefined : EMPTY_OVERVIEW)
  const derived = useMemo(
    () => (data ? deriveSessionView(data, events.data, overviewData) : undefined),
    [data, events.data, overviewData],
  )
  if (detail.isPending) {
    return <PendingSessionHistory sessionId={sessionId} initialPrompt={initialPrompt} />
  }

  if (detail.isError && !detail.data) {
    return (
      <EmptyState title="Couldn't load this session">
        <Button
          className="mt-2 min-h-10"
          variant="primary"
          onClick={() => void detail.refetch()}
        >
          Retry
        </Button>
      </EmptyState>
    )
  }

  if (!derived) return null // unreachable: derived exists whenever detail.data does
  const { session, messages } = detail.data
  const {
    transcriptEvents,
    displayEvents,
    acpThinking,
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
  const visibleSessionError = sessionError || live?.error?.trim() || ''
  const sessionErrorContext = [session.model_provider, session.model].filter(Boolean).join(' · ')
  const isACP = session.runtime === 'acp'
  // Covers turns started elsewhere (parent-triggered, or refresh mid-turn).
  const sessionRunning = queue.sessionRunning
  const pendingSteer = session.pending_steer_message
  const initialUser = pendingOptimisticUserMessage(messages, initialPrompt ?? null)
  const optimisticUser = isACP && live ? liveOptimisticUserMessage(live) : initialUser
  const empty = messages.length === 0 && transcriptEvents.length === 0 && !optimisticUser && !visibleSessionError && !sessionRunning
  // ACP turns stream through events. While the request is active, the local
  // send time is the turn boundary; replayed user rows can be timestamped after
  // early live events and would otherwise fold those events into the prior turn.
  const transcriptMessages = optimisticTranscriptMessages(messages, optimisticUser)
  const goalStatusVisible = goalActive
  const canContinueFromInlineError =
    isACP && !sessionRunning && !streaming && (!live || Boolean(live.error)) && !hasBlockingPendingPermission
  const continueErrorAction: SessionErrorAction | undefined =
    canContinueFromInlineError ? {
      label: 'Continue',
      icon: <Play size={13} aria-hidden />,
      onClick: () => handleSend('Continue'),
      title: 'Continue this thread',
    } : undefined
  const errorNotice = visibleSessionError ? (
    <SessionErrorNotice
      message={visibleSessionError}
      context={sessionError ? sessionErrorContext : undefined}
      className="mt-5"
      action={continueErrorAction}
    />
  ) : null

  return (
    <FileReaderLinkProvider sessionId={session.id} documentPath={session.runtime_ref?.cwd ? `${session.runtime_ref.cwd}/` : undefined} onOpen={sidePanel.openFile}>
      <PreviewLinkProvider onOpen={sidePanel.openPreview}>
        {/* Phone: the closed side panel slides off to the right (translateX 100%);
            clip horizontal overflow so it can't be revealed by scrolling. */}
        <FileDropScope ref={sidePanel.containerRef} className="relative flex h-full max-sm:overflow-x-clip">
          <SessionTitlebar
            session={session}
            isMobile={isMobile}
            panel={sidePanel}
            sideChatAvailable={sideChatAvailable}
            header={header}
            overviewLabel={details ? 'Details' : undefined}
          />
          {/* Phone: the open panel covers the chat full-width, so the only
              non-panel area left is the title bar. This catches taps on its empty
              space (the header controls sit above it) to dismiss the panel. */}
          {isMobile && sidePanel.open ? (
            <div className="fixed inset-0 z-scrim" aria-hidden onClick={sidePanel.close} />
          ) : null}
          {sidePanel.resizing ? <div className="fixed inset-0 z-modal cursor-col-resize" aria-hidden /> : null}

          <div className="relative h-full min-w-0 flex-1">
            <div
              ref={attachScroll}
              className="scrollbar-quiet relative z-0 h-full overflow-y-auto"
              onScroll={onThreadScroll}
              onClickCapture={onThreadClickCapture}
            >
              <div
                ref={threadFind.rootRef}
                className={`${THREAD_COLUMN_CLASS} pt-6`}
                style={{ paddingBottom: transcriptBottomPadding }}
              >
                {chat ? (
                  <>
                    {chat({ messages: transcriptMessages, events: displayEvents, scrollRef, paging, working: sessionRunning, threads: spawnedThreads, send: handleSend, showDetails })}
                    {errorNotice}
                  </>
                ) : empty ? (
                  <EmptyState title="Start the conversation">
                    <p>Messages stream in live as your assistant thinks and works.</p>
                  </EmptyState>
                ) : (
                  <>
                    <Transcript
                      scrollRef={scrollRef}
                      key={session.id}
                      messages={transcriptMessages}
                      events={displayEvents}
                      sessionId={session.id}
                      groupTurns={isACP}
                      working={sessionRunning}
                      findActive={threadFind.open && Boolean(threadFind.query.trim())}
                      revealSeq={revealedMessageSeq}
                      errorAction={visibleSessionError ? undefined : continueErrorAction}
                      onArtifactPrompt={handleSend}
                      paging={paging}
                      tail={
                        isACP ? (
                          <>
                            {pendingSteer ? <PendingSteerBubble prompt={pendingSteer} /> : null}
                            <SessionLivenessIndicator
                              agent={session.runtime_ref?.agent}
                              running={sessionRunning}
                              thinking={acpThinking}
                              activeOperation={detail.data?.acp_active_operation}
                              updatedAt={session.updated_at}
                              lastActivityAt={latestEventTimeISO(lastSessionEventAt, live?.at)}
                            />
                          </>
                        ) : live ? (
                          <div className="flex flex-col gap-2">
                            <motion.div
                              className="w-full"
                              initial={{ opacity: 0, y: 8 }}
                              animate={{ opacity: 1, y: 0 }}
                              transition={{ type: 'spring', stiffness: 380, damping: 30 }}
                            >
                              <UserBubble
                                text={live.user}
                                createdAt={live.at}
                                contexts={live.contexts}
                                attachments={live.attachments}
                                attachmentSessionId={sessionId}
                              />
                            </motion.div>
                            <ThinkingBlock text={live.reasoning} pending={streaming} />
                            <ToolCalls
                              calls={live.tools.map((tool) => ({
                                ...tool,
                                pending: streaming && tool.result === undefined,
                              }))}
                              onArtifactPrompt={handleSend}
                            />
                            {live.assistant ? (
                              <MessageMarkdown text={live.assistant} />
                            ) : streaming ? (
                              <p className="animate-pulse text-sm text-ink-3">Thinking…</p>
                            ) : null}
                          </div>
                        ) : null
                      }
                    />
                    {errorNotice}
                  </>
                )}
              </div>
            </div>
            {chat ? null : (
              <ThreadOutline
                messages={transcriptMessages}
                events={displayEvents}
                scrollRef={scrollRef}
                onSelect={jumpToMessage}
              />
            )}
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
              <VoiceMode voice={voice} face={voice.bot?.avatar ?? null} />
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
                    placeholder={placeholder}
                    streaming={sessionRunning}
                    commands={derived.agentSession?.commands ?? undefined}
                    optionsSlot={<NativeModelOptions options={derived.agentSession?.config_options} running={sessionRunning} pending={config.isPending} onChange={(id, value) => config.mutate({ id, value })} />}
                    chat={chat !== undefined}
                    planAvailable={planAvailable}
                    planModeActive={Boolean(live?.planRequested) || planActive}
                    goalControlVisible
                    goalAvailable={goalAvailable}
                    goalEngaged={goalRequested || goalActive}
                    queuedPrompts={queue.queuedPrompts}
                    steerDisabled={queue.steerDisabled}
                    draftStorageKey={`${SESSION_DRAFT_KEY_PREFIX}${session.id}`}
                    fileRoot={session.runtime_ref?.cwd}
                    attachmentSessionId={session.id}
                    contexts={composerContexts.contexts}
                    onRemoveContext={composerContexts.removeContext}
                    onReplaceContexts={composerContexts.replaceContexts}
                    onSend={handleSend}
                    onQueuePrompt={queue.onQueuePrompt}
                    onStop={stopSession}
                    onClearGoal={clearGoal}
                    onVoice={voiceAvailable && voice.phase === 'off' ? voice.start : undefined}
                    voiceActive={voice.phase !== 'off'}
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
          <SidePanelDrawer panel={sidePanel} isMobile={isMobile}>
            <SidePanel
              session={session}
              progress={panelProgress}
              subagents={providerSubagents}
              spawnedThreads={spawnedThreads}
              working={sessionRunning}
              panel={sidePanel}
              sideChatAvailable={sideChatAvailable}
              sideChatEvents={sideChatEvents}
              overview={details?.({ agentSession: derived.agentSession, working: sessionRunning, threads: spawnedThreads, subagents: providerSubagents })}
              onAddBrowserAnnotation={composerContexts.addBrowserAnnotation}
              onUploadAttachment={(file) => uploadSessionAttachment(session.id, file)}
              onSend={handleSend}
              onQueuePrompt={queue.onQueuePrompt}
              onQueueAction={queue.onQueueAction}
              onSendSideChat={handleSideChatSend}
            />
          </SidePanelDrawer>
        </FileDropScope>
      </PreviewLinkProvider>
    </FileReaderLinkProvider>
  )
}
