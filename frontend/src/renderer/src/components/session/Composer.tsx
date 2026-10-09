import { ArrowUp, AudioLines, ListChecks, LoaderCircle, Mic, Plus, Square, Target, X } from 'lucide-react'
import { AnimatePresence, motion } from 'motion/react'
import { type ClipboardEvent, type ReactNode, useCallback, useEffect, useRef, useState } from 'react'
import { FileDropOverlay, useFileDropTarget } from '@/components/ui/FileDrop'
import { IconButton } from '@/components/ui/IconButton'
import { SwitchRow } from '@/components/ui/Switch'
import { useToast } from '@/components/ui/toast'
import { composerPasteFiles } from '@/components/session/composerPasteFiles'
import { subscribeComposerDraft } from '@/components/session/composerDraftChanges'
import { apiBaseUrl, isLocalBackendUrl } from '@/lib/api/client'
import type { AgentSessionCommand, Attachment, QueuedMessage } from '@/lib/api/types'
import type { ComposerContext, SendMessageHandler } from '@/lib/sendMessage'
import { Popover } from '@/components/ui/Popover'
import { useDictation } from '@/lib/hooks/useDictation'
import { dictationText } from '@/lib/dictationText'
import { DictationControls } from '@/components/session/DictationControls'
import { ComposerAttachmentInput, ComposerAttachmentList, ComposerAttachmentMenuRow } from './ComposerAttachments'
import { MentionSuggestions, MentionTextarea, useMentionInput } from './MentionInput'
import { QueuedPromptList } from './QueuedPromptList'
import { ComposerFrame } from './ComposerFrame'
import { ContextChip } from './ContextChip'
import { GoalChip, GoalUnsupportedRow } from './GoalControls'
import { useComposerAttachments } from './useComposerAttachments'
import type { ComposerDraftStorage } from './useComposerDraft'

// Composer in the agent-council style: borderless auto-growing textarea on a
// raised card, toolbar row beneath with the send/stop action. The card is the
// focus surface — while focused, a rainbow conic ring circles the card.
export function ComposerCard({
  streaming,
  autoFocus,
  placeholder = 'Ask anything, or hand your assistant a task…',
  disabled = false,
  planAvailable = false,
  planModeActive = false,
  goalControlVisible = false,
  goalAvailable = false,
  goalEngaged = false,
  sendWhileStreaming = false,
  draftStorageKey,
  draftStorage = 'session',
  clearTiming = 'resolved',
  leftSlot,
  chat = false,
  fileRoot,
  attachmentSessionId,
  contexts = [],
  onSend,
  onQueuePrompt,
  commands,
  optionsSlot,
  onStop,
  onClearGoal,
  onVoice,
  voiceActive = false,
  onUploadAttachment,
  onRemoveContext,
  onReplaceContexts,
  onTextChange,
}: {
  streaming: boolean
  autoFocus?: boolean
  placeholder?: string
  disabled?: boolean
  planAvailable?: boolean
  planModeActive?: boolean
  goalControlVisible?: boolean
  goalAvailable?: boolean
  goalEngaged?: boolean
  sendWhileStreaming?: boolean
  draftStorageKey?: string
  draftStorage?: ComposerDraftStorage
  clearTiming?: 'immediate' | 'resolved' | 'never'
  /** leading toolbar content (e.g. the new-thread runtime/project controls) */
  leftSlot?: ReactNode
  /** messenger layout for bot chats: one row, no options menu */
  chat?: boolean
  /** server-side directory the @-mention file picker indexes (a project path,
      session cwd, or '' for the workspace root). undefined disables files */
  fileRoot?: string
  attachmentSessionId?: string
  /** text selections and browser annotations attached to the next message */
  contexts?: ComposerContext[]
  onSend: SendMessageHandler
  onQueuePrompt?: SendMessageHandler
  commands?: AgentSessionCommand[]
  optionsSlot?: ReactNode
  onStop?: () => void
  /** stops the goal auto-continuation loop server-side */
  onClearGoal?: () => void
  onVoice?: () => void
  voiceActive?: boolean
  onUploadAttachment?: (file: File) => Promise<Attachment>
  onRemoveContext?: (id: string) => void
  onReplaceContexts?: (contexts: ComposerContext[]) => void
  onTextChange?: (text: string) => void
}) {
  const [optionsOpen, setOptionsOpen] = useState(false)
  const [planModeOverride, setPlanModeOverride] = useState<boolean | null>(null)
  const [goalRequested, setGoalRequestedState] = useState(false)
  const goalRequestedRef = useRef(false)
  const contextsRef = useRef(contexts)
  const clearGenerationRef = useRef(0)
  contextsRef.current = contexts
  const fileInputRef = useRef<HTMLInputElement>(null)
  const planToggleDisabled = disabled || !planAvailable
  const goalToggleDisabled = disabled || !goalAvailable
  const planModeOn = planAvailable && (planModeOverride ?? planModeActive)
  const goalModeOn = goalAvailable && (goalRequested || goalEngaged)
  const showGoalChip = goalModeOn
  const mention = useMentionInput({
    commands,
    fileRoot,
    disabled,
    storageKey: draftStorageKey,
    storage: draftStorage,
    onTextChange,
  })
  const attachmentDraft = useComposerAttachments({
    storageKey: draftStorageKey,
    storage: draftStorage,
    disabled,
    onUploadAttachment,
  })
  const canSendWhileStreaming = streaming && sendWhileStreaming
  const attachmentBusy = attachmentDraft.busy
  const hasNonTextDraftContent =
    attachmentDraft.files.length > 0 || attachmentDraft.uploaded.length > 0 || contexts.length > 0
  const hasSendableDraft = (messageEmpty: boolean) => !messageEmpty || hasNonTextDraftContent
  const hasDraftContent = hasSendableDraft(mention.isEmpty)
  const showVoiceButton = !hasDraftContent && Boolean(onVoice)
  const actionLabel = showVoiceButton ? 'Voice mode' : 'Send message'
  const submitDisabled = !hasDraftContent || disabled || attachmentBusy || (streaming && !canSendWhileStreaming)
  const showStopButton = !voiceActive && streaming && onStop && (!sendWhileStreaming || !hasDraftContent)
  const dictation = useDictation({
    identity: `${draftStorage}:${draftStorageKey ?? ''}`,
    disabled: disabled || voiceActive,
    onComplete: (transcript, send) => {
      const draft = mention.currentDraft()
      const start = mention.textareaRef.current?.selectionStart ?? draft.text.length
      const end = mention.textareaRef.current?.selectionEnd ?? start
      const value = mention.insertText(dictationText(draft.text, start, end, transcript))
      if (send) {
        void submit(value)
      }
    },
  })

  const setGoalRequested = useCallback((next: boolean) => {
    if (goalRequestedRef.current === next) return
    goalRequestedRef.current = next
    setGoalRequestedState(next)
  }, [])

  useEffect(() => {
    if (!planAvailable) setPlanModeOverride(null)
  }, [planAvailable])

  useEffect(() => {
    setPlanModeOverride((override) => (override === planModeActive ? null : override))
  }, [planModeActive])

  useEffect(() => {
    if (!goalAvailable || goalEngaged) setGoalRequested(false)
  }, [goalEngaged, goalAvailable, setGoalRequested])

  const toast = useToast()
  const { dropTargetRef, dragging: draggingFiles } = useFileDropTarget<HTMLDivElement>({
    disabled,
    onDrop: (files, folders) => {
      attachmentDraft.addFiles(files)
      if (folders.length === 0) {
        return
      }
      const pathFor = isLocalBackendUrl(apiBaseUrl()) ? window.jaz?.pathForFile : undefined
      if (pathFor) {
        mention.insertPaths(folders.map((folder) => pathFor(folder)))
      } else {
        toast('Folders attach only in the desktop app with a local backend', 'danger')
      }
    },
  })

  const onPasteCapture = (event: ClipboardEvent<HTMLDivElement>) => {
    if (disabled || event.target !== mention.textareaRef.current) {
      return
    }
    const files = composerPasteFiles(event.clipboardData)
    if (files.length === 0) return
    event.preventDefault()
    attachmentDraft.addFiles(files)
  }

  const setPlanMode = useCallback((next: boolean) => {
    setPlanModeOverride(next === planModeActive ? null : next)
  }, [planModeActive])

  const togglePlanMode = () => {
    if (planToggleDisabled) return
    setPlanMode(!planModeOn)
  }

  // An engaged goal is cleared server-side; a not-yet-sent request is disarmed.
  const turnGoalOff = () => {
    if (goalEngaged) onClearGoal?.()
    else setGoalRequested(false)
  }

  const toggleGoalRequested = () => {
    if (goalToggleDisabled) return
    if (goalModeOn) turnGoalOff()
    else setGoalRequested(true)
  }

  useEffect(() => {
    if (planToggleDisabled) return
    const onKeyDown = (event: KeyboardEvent) => {
      if (
        event.defaultPrevented ||
        event.key !== 'Tab' ||
        !event.shiftKey ||
        event.metaKey ||
        event.ctrlKey ||
        event.altKey ||
        document.querySelector('[role="dialog"][aria-modal="true"]')
      ) {
        return
      }
      event.preventDefault()
      setPlanMode(!planModeOn)
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [planModeOn, planToggleDisabled, setPlanMode])

  const clearDraft = () => {
    const generation = ++clearGenerationRef.current
    const sent = {
      editor: mention.currentDraft(),
      attachments: attachmentDraft.currentAttachments(),
      contexts: contextsRef.current,
      goalRequested: goalRequestedRef.current,
    }
    mention.reset()
    attachmentDraft.clearAttachments()
    contextsRef.current = []
    onReplaceContexts?.([])
    setGoalRequested(false)
    let unchanged = true
    const dispose = subscribeComposerDraft(draftStorage, draftStorageKey, () => {
      unchanged = false
    })
    const restore = () => {
      if (
        !unchanged ||
        generation !== clearGenerationRef.current ||
        mention.currentDraft().text !== '' ||
        attachmentDraft.currentAttachments().length > 0 ||
        contextsRef.current.length > 0 ||
        goalRequestedRef.current
      ) {
        return
      }
      mention.restore(sent.editor)
      attachmentDraft.replaceAttachments(sent.attachments)
      contextsRef.current = sent.contexts
      onReplaceContexts?.(sent.contexts)
      setGoalRequested(sent.goalRequested)
    }
    return { restore, dispose }
  }

  const submit = async (value = mention.value(), handler = onSend) => {
    // Tokens expand on the way out: tagged paths become absolute, skill
    // references pass through for the agent's skill catalog to resolve.
    const trimmed = value.trim()
    if (
      !hasSendableDraft(trimmed === '') ||
      disabled ||
      attachmentBusy ||
      (streaming && !canSendWhileStreaming)
    ) {
      return
    }
    const send = () =>
      handler(trimmed, {
        planRequested: planModeOn,
        goalRequested: goalModeOn,
        files: attachmentDraft.files,
        attachments: attachmentDraft.uploaded,
        ...(contexts.length > 0 ? { contexts } : {}),
      })
    let rollback: ReturnType<typeof clearDraft> | undefined
    try {
      const pending = send()
      if (clearTiming === 'immediate') {
        rollback = clearDraft()
      }
      await pending
      if (clearTiming === 'resolved') {
        clearDraft().dispose()
      }
    } catch {
      rollback?.restore()
    } finally {
      rollback?.dispose()
    }
  }

  const textarea = (
    <MentionTextarea
      mention={mention}
      placeholder={placeholder}
      disabled={disabled}
      readOnly={dictation.phase !== null}
      autoFocus={autoFocus}
      onKeyDown={(e) => {
        if (
          e.key === 'Tab' && !e.shiftKey && !e.metaKey && !e.ctrlKey && !e.altKey &&
          streaming && onQueuePrompt && hasDraftContent && !dictation.phase
        ) {
          e.preventDefault()
          void submit(mention.value(), onQueuePrompt)
        }
        if (e.key === 'Enter' && !e.shiftKey) {
          e.preventDefault()
          if (dictation.phase) {
            void dictation.stop()
          } else {
            void submit()
          }
        }
      }}
    />
  )

  const chips = (
    <AnimatePresence initial={false}>
      {planModeOn ? (
        <motion.div
          key="plan-chip"
          initial={{ opacity: 0, scale: 0.8, filter: 'blur(4px)' }}
          animate={{ opacity: 1, scale: 1, filter: 'blur(0px)' }}
          exit={{ opacity: 0, scale: 0.8, filter: 'blur(4px)' }}
          transition={{ type: 'spring', duration: 0.3, bounce: 0 }}
          className="group flex h-8 shrink-0 items-center gap-1 rounded-full pr-2.5 pl-1 text-[13px] font-medium text-ink-2 transition-colors duration-150 hover:bg-surface-2 hover:text-ink"
        >
          <IconButton
            variant="ghost"
            size="xs"
            aria-label="Remove plan mode"
            title="Remove plan mode"
            disabled={disabled}
            className="grid"
            onClick={() => setPlanMode(false)}
          >
            <ListChecks
              size={13}
              className="col-start-1 row-start-1 transition-opacity group-hover:opacity-0 group-focus-within:opacity-0"
            />
            <X
              size={13}
              className="col-start-1 row-start-1 opacity-0 transition-opacity group-hover:opacity-100 group-focus-within:opacity-100"
            />
          </IconButton>
          <span>Plan</span>
        </motion.div>
      ) : null}
      {showGoalChip ? (
        <GoalChip
          active={goalEngaged}
          requested={goalRequested || goalEngaged}
          disabled={disabled}
          onRemove={turnGoalOff}
        />
      ) : null}
    </AnimatePresence>
  )

  return (
    <div
      ref={dropTargetRef}
      className="relative"
      onPasteCapture={onPasteCapture}
      onKeyDownCapture={(event) => {
        if (dictation.phase && event.key === 'Escape') {
          event.preventDefault()
          event.stopPropagation()
          dictation.cancel()
        }
      }}
    >
      <FileDropOverlay visible={draggingFiles} />
      {!dictation.phase ? <MentionSuggestions mention={mention} placement="above" /> : null}
      {/* The whole card is a click target for the textarea. */}
      <ComposerFrame
        chat={chat}
        className={`flex cursor-text flex-col gap-1.5 ${draggingFiles ? 'shadow-[0_0_0_1px_var(--color-primary),0_10px_35px_rgba(0,0,0,0.16)]' : ''}`}
        onClick={(e) => {
          if ((e.target as HTMLElement).closest('button, textarea, input')) return
          mention.textareaRef.current?.focus()
        }}
      >
        <ComposerAttachmentInput
          disabled={disabled}
          inputRef={fileInputRef}
          onAddFiles={attachmentDraft.addFiles}
        />
        {contexts.length > 0 ? (
          <div className="flex flex-wrap gap-1 px-1.5 pt-0.5">
            {contexts.map((context, index) => (
              <ContextChip
                key={context.id}
                index={index}
                context={context}
                onRemove={onRemoveContext ? () => onRemoveContext(context.id) : undefined}
              />
            ))}
          </div>
        ) : null}
        <ComposerAttachmentList
          attachments={attachmentDraft.attachments}
          attachmentSessionId={attachmentSessionId}
          onRemove={attachmentDraft.removeAttachment}
        />
        {chat ? <div className="flex flex-wrap gap-1.5 empty:hidden">{chips}</div> : <div>{textarea}</div>}
        {dictation.error ? (
          <div className="flex items-center gap-2 pl-2 text-sm text-danger" role="alert">
            <span className="min-w-0 flex-1">{dictation.error}</span>
            <IconButton className="size-10" aria-label="Dismiss dictation error" onClick={dictation.dismissError}><X size={16} /></IconButton>
          </div>
        ) : null}
        {/* Chat is messenger style: the textarea shares the controls' row, its
            bottom padding centring its last line on them, and isolate keeps
            its z-index under the dictation overlay. */}
        <div className={`relative flex ${chat ? 'items-end gap-1' : 'items-center justify-between gap-2.5 max-sm:items-end'}`}>
          {chat ? (
            <div className="isolate min-w-0 flex-1 pb-1.5">{textarea}</div>
          ) : (
            // Phone: the new-thread controls (agent, model, project, worktree)
            // outgrow one row, so let them wrap and keep send pinned bottom-right.
            <div className="flex min-w-0 items-center gap-1.5 max-sm:flex-1 max-sm:flex-wrap" inert={dictation.phase !== null}>
              <Popover
                open={optionsOpen}
                onClose={() => setOptionsOpen(false)}
                trigger={
                  <IconButton
                    variant="ghost"
                    size="md"
                    aria-haspopup="menu"
                    aria-expanded={optionsOpen}
                    aria-label="Composer options"
                    title="Composer options"
                    disabled={disabled}
                    onClick={() => setOptionsOpen((value) => !value)}
                  >
                    <Plus
                      size={16}
                      className={`transition-transform duration-200 ease-out ${
                        optionsOpen ? 'rotate-45' : ''
                      }`}
                    />
                  </IconButton>
                }
              >
                <ComposerAttachmentMenuRow
                  disabled={disabled}
                  onChoose={() => {
                    setOptionsOpen(false)
                    fileInputRef.current?.click()
                  }}
                />
                {planAvailable ? (
                  <SwitchRow
                    icon={<ListChecks size={13} className="shrink-0" />}
                    label="Plan"
                    checked={planModeOn}
                    disabled={disabled}
                    onChange={togglePlanMode}
                  />
                ) : null}
                {goalControlVisible ? (
                  goalAvailable ? (
                    <SwitchRow
                      icon={<Target size={13} className="shrink-0" />}
                      label="Goal"
                      checked={goalEngaged || goalRequested}
                      disabled={goalToggleDisabled}
                      onChange={toggleGoalRequested}
                    />
                  ) : (
                    <GoalUnsupportedRow />
                  )
                ) : null}
                {optionsSlot}
              </Popover>
              {leftSlot}
              {chips}
            </div>
          )}
          <div className="flex min-h-10 shrink-0 items-center gap-2" inert={dictation.phase !== null}>
            {dictation.showButton && !voiceActive ? (
              <IconButton
                variant="ghost"
                size="lg"
                className="relative after:absolute after:-inset-0.5"
                aria-label="Dictate"
                title={dictation.availability.available ? 'Dictate' : dictation.availability.reason || 'Checking dictation availability…'}
                disabled={disabled || voiceActive || !dictation.availability.available}
                onClick={() => {
                  setOptionsOpen(false)
                  mention.textareaRef.current?.focus()
                  void dictation.start()
                }}
              >
                <Mic size={16} />
              </IconButton>
            ) : null}
            {streaming && onQueuePrompt && hasDraftContent && !voiceActive ? (
              <button
                type="button"
                title="Queue message (Tab)"
                aria-label="Queue message"
                disabled={submitDisabled}
                onClick={() => void submit(mention.value(), onQueuePrompt)}
                className="h-10 rounded-full px-2 text-[13px] text-ink-2 transition-colors hover:bg-surface-2 disabled:opacity-50"
              >
                Queue
              </button>
            ) : null}
            {showStopButton ? (
              <IconButton
                variant="primary"
                size="md"
                className="relative after:absolute after:-inset-1"
                aria-label="Stop response"
                title="Stop response"
                onClick={onStop}
              >
                <Square size={12} fill="currentColor" strokeWidth={0} />
              </IconButton>
            ) : !voiceActive || hasDraftContent ? (
              <IconButton
                variant="primary"
                size="md"
                className="relative after:absolute after:-inset-1"
                aria-label={actionLabel}
                title={actionLabel}
                disabled={showVoiceButton ? disabled || Boolean(dictation.phase) : submitDisabled}
                onClick={showVoiceButton ? onVoice : () => void submit()}
              >
                {showVoiceButton ? <AudioLines size={16} /> : <ArrowUp size={16} />}
              </IconButton>
            ) : null}
          </div>
          {/* Opaque, so it hides the controls and, in chat, the draft while the
              textarea keeps focus for Enter and Escape. */}
          {dictation.phase ? (
            <div className="absolute inset-0 flex flex-col justify-end bg-surface">
              <DictationControls dictation={dictation} canSend={!disabled && !attachmentBusy && (!streaming || canSendWhileStreaming)} queue={false} />
            </div>
          ) : null}
        </div>
      </ComposerFrame>
    </div>
  )
}

export function Composer({
  streaming,
  disabled,
  placeholder,
  planAvailable,
  planModeActive,
  goalControlVisible,
  goalAvailable,
  goalEngaged,
  queuedPrompts = [],
  steerDisabled,
  draftStorageKey,
  fileRoot,
  attachmentSessionId,
  contexts,
  onSend,
  onQueuePrompt,
  commands,
  optionsSlot,
  chat,
  onStop,
  onClearGoal,
  onVoice,
  voiceActive = false,
  onUploadAttachment,
  onRemoveContext,
  onReplaceContexts,
  onSteerQueuedPrompt,
  onDeleteQueuedPrompt,
  onEditQueuedPrompt,
  onReorderQueuedPrompts,
}: {
  streaming: boolean
  disabled?: boolean
  placeholder?: string
  planAvailable?: boolean
  planModeActive?: boolean
  goalControlVisible?: boolean
  goalAvailable?: boolean
  goalEngaged?: boolean
  queuedPrompts?: QueuedMessage[]
  steerDisabled?: boolean
  draftStorageKey?: string
  /** directory the @-mention file picker indexes; undefined disables files */
  fileRoot?: string
  attachmentSessionId?: string
  contexts?: ComposerContext[]
  onSend: SendMessageHandler
  onQueuePrompt?: SendMessageHandler
  commands?: AgentSessionCommand[]
  optionsSlot?: ReactNode
  chat?: boolean
  onStop: () => void
  onClearGoal?: () => void
  onVoice?: () => void
  voiceActive?: boolean
  onUploadAttachment?: (file: File) => Promise<Attachment>
  onRemoveContext?: (id: string) => void
  onReplaceContexts?: (contexts: ComposerContext[]) => void
  onSteerQueuedPrompt?: (id: string) => void
  onDeleteQueuedPrompt?: (id: string) => void
  onEditQueuedPrompt?: (id: string, text: string) => void
  onReorderQueuedPrompts?: (ids: string[]) => void
}) {
  return (
    <>
      <AnimatePresence initial={false}>
        {queuedPrompts.length > 0 &&
        onSteerQueuedPrompt &&
        onDeleteQueuedPrompt &&
        onEditQueuedPrompt &&
        onReorderQueuedPrompts ? (
          <QueuedPromptList
            prompts={queuedPrompts}
            steerDisabled={steerDisabled}
            onSteer={onSteerQueuedPrompt}
            onDelete={onDeleteQueuedPrompt}
            onEdit={onEditQueuedPrompt}
            onReorder={onReorderQueuedPrompts}
          />
        ) : null}
      </AnimatePresence>
      <ComposerCard
        streaming={streaming}
        disabled={disabled}
        placeholder={placeholder}
        planAvailable={planAvailable}
        planModeActive={planModeActive}
        goalControlVisible={goalControlVisible}
        goalAvailable={goalAvailable}
        goalEngaged={goalEngaged}
        sendWhileStreaming
        clearTiming="immediate"
        draftStorageKey={draftStorageKey}
        draftStorage="local"
        fileRoot={fileRoot}
        attachmentSessionId={attachmentSessionId}
        contexts={contexts}
        onSend={onSend}
        onQueuePrompt={onQueuePrompt}
        commands={commands}
        optionsSlot={optionsSlot}
        chat={chat}
        onStop={onStop}
        onClearGoal={onClearGoal}
        onVoice={onVoice}
        voiceActive={voiceActive}
        onUploadAttachment={onUploadAttachment}
        onRemoveContext={onRemoveContext}
        onReplaceContexts={onReplaceContexts}
      />
    </>
  )
}

export function PlanDecisionCard({
  disabled,
  pending,
  onImplement,
  onClarify,
}: {
  disabled?: boolean
  pending?: boolean
  onImplement: () => void
  onClarify: (text: string) => void
}) {
  const [clarifying, setClarifying] = useState(false)
  const [text, setText] = useState('')
  const inputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    if (clarifying) inputRef.current?.focus()
  }, [clarifying])

  const submitClarification = () => {
    const trimmed = text.trim()
    if (!trimmed || disabled || pending) return
    onClarify(trimmed)
    setText('')
    setClarifying(false)
  }

  return (
    <div className="rounded-[12px] bg-surface p-2.5">
      <p className="px-2 pt-0.5 pb-2 text-sm font-medium text-ink">Implement this plan?</p>
      <div className="flex flex-col gap-0.5">
        <motion.button
          type="button"
          disabled={disabled || pending}
          onClick={onImplement}
          whileTap={{ scale: 0.99 }}
          className="flex h-9 w-full cursor-pointer items-center gap-2.5 rounded-full px-3 text-left text-sm font-medium text-ink transition-colors duration-150 hover:bg-primary-soft disabled:cursor-default disabled:opacity-60"
        >
          {pending ? (
            <LoaderCircle size={15} className="shrink-0 animate-spin text-primary" />
          ) : null}
          {pending ? 'Starting implementation…' : 'Yes, implement this plan'}
        </motion.button>

        {clarifying ? (
          // the "no" row, morphed in place into the clarification field
          <motion.div
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            transition={{ duration: 0.15, ease: 'easeOut' }}
            className="flex h-9 items-center gap-2 rounded-full bg-bg pr-1 pl-3"
          >
            <input
              ref={inputRef}
              value={text}
              disabled={disabled}
              placeholder="What should change in this plan?"
              className="h-full min-w-0 flex-1 bg-transparent text-sm text-ink placeholder:text-ink-3 disabled:cursor-default"
              onChange={(e) => setText(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter') {
                  e.preventDefault()
                  submitClarification()
                }
                if (e.key === 'Escape') {
                  e.preventDefault()
                  setClarifying(false)
                  setText('')
                }
              }}
            />
            <IconButton
              variant="primary"
              size="sm"
              aria-label="Send clarification"
              title="Send clarification"
              disabled={!text.trim() || disabled}
              onClick={submitClarification}
            >
              <ArrowUp size={14} />
            </IconButton>
          </motion.div>
        ) : (
          <motion.button
            type="button"
            disabled={disabled || pending}
            onClick={() => setClarifying(true)}
            whileTap={{ scale: 0.99 }}
            className="flex h-9 w-full cursor-pointer items-center gap-2.5 rounded-full px-3 text-left text-sm text-ink-2 transition-colors duration-150 hover:bg-surface-2 hover:text-ink disabled:cursor-default disabled:opacity-60"
          >
            <X size={15} className="shrink-0 text-ink-3" />
            No, I'll clarify first
          </motion.button>
        )}
      </div>
    </div>
  )
}
