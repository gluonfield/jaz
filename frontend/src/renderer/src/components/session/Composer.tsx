import { ArrowUp, AudioLines, ListChecks, LoaderCircle, Plus, Square, X } from 'lucide-react'
import { AnimatePresence, motion, useReducedMotion } from 'motion/react'
import { type ClipboardEvent, type ReactNode, useCallback, useEffect, useRef, useState } from 'react'
import { FileDropOverlay, useFileDropTarget } from '@/components/ui/FileDrop'
import { IconButton } from '@/components/ui/IconButton'
import { clipboardFiles } from '@/components/ui/fileTransfer'
import type { Attachment, QueuedMessage } from '@/lib/api/types'
import type { ComposerContext, SendMessageHandler } from '@/lib/sendMessage'
import { Popover } from '@/components/ui/Popover'
import { RAINBOW_BEAM } from '@/components/ui/rainbow'
import { useEffectsEnabled } from '@/lib/appearance'
import { ComposerAttachmentInput, ComposerAttachmentList, ComposerAttachmentMenuRow } from './ComposerAttachments'
import { MentionSuggestions, MentionTextarea, useMentionInput } from './MentionInput'
import { QueuedPromptList } from './QueuedPromptList'
import { ContextChip } from './ContextChip'
import { GoalChip, GoalMenuToggle, GoalUnsupportedRow } from './GoalControls'
import { useComposerAttachments } from './useComposerAttachments'
import type { ComposerDraftStorage } from './useComposerDraft'

function PlanMenuToggle({
  checked,
  disabled,
  onToggle,
}: {
  checked: boolean
  disabled?: boolean
  onToggle: () => void
}) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      disabled={disabled}
      onClick={onToggle}
      className={`flex h-7 w-full items-center gap-2 rounded-full px-2.5 text-left text-[13px] transition-colors duration-150 enabled:hover:bg-surface-2 disabled:cursor-default disabled:opacity-50 ${
        checked ? 'text-ink' : 'text-ink-2'
      }`}
    >
      <span className="min-w-0 flex-1 truncate">Plan</span>
      {/* mirrors the shared Switch primitive: spring-driven layout thumb, no
          forbidden transition-all */}
      <span
        aria-hidden
        className={`relative inline-flex h-4 w-7 shrink-0 items-center rounded-full transition-colors duration-150 ${
          checked ? 'bg-primary' : 'bg-ink/20'
        }`}
      >
        <motion.span
          layout
          transition={{ type: 'spring', stiffness: 500, damping: 34 }}
          className={`absolute size-3 rounded-full ${
            checked ? 'right-0.5 bg-on-primary' : 'left-0.5 bg-ink/60'
          }`}
        />
      </span>
    </button>
  )
}

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
  goalActive = false,
  queueWhenStreaming = false,
  translucent = false,
  draftStorageKey,
  draftStorage = 'session',
  clearOnSend = true,
  leftSlot,
  fileRoot,
  contexts = [],
  onSend,
  onStop,
  onVoice,
  onUploadAttachment,
  onRemoveContext,
  onClearContexts,
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
  goalActive?: boolean
  queueWhenStreaming?: boolean
  /** let a backdrop (e.g. the welcome particle field) read through the card */
  translucent?: boolean
  draftStorageKey?: string
  draftStorage?: ComposerDraftStorage
  clearOnSend?: boolean
  /** leading toolbar content (e.g. the new-thread runtime/project controls) */
  leftSlot?: ReactNode
  /** server-side directory the @-mention file picker indexes (a project path,
      session cwd, or '' for the workspace root). undefined disables files */
  fileRoot?: string
  /** text selections and browser annotations attached to the next message */
  contexts?: ComposerContext[]
  onSend: SendMessageHandler
  onStop?: () => void
  onVoice?: () => void
  onUploadAttachment?: (file: File) => Promise<Attachment>
  onRemoveContext?: (id: string) => void
  onClearContexts?: () => void
  onTextChange?: (text: string) => void
}) {
  const [focused, setFocused] = useState(false)
  const [optionsOpen, setOptionsOpen] = useState(false)
  const [planModeOverride, setPlanModeOverride] = useState<boolean | null>(null)
  const [goalRequested, setGoalRequested] = useState(false)
  const fileInputRef = useRef<HTMLInputElement>(null)
  const reducedMotion = useReducedMotion()
  // With effects off, drop the rainbow focus comet for a calm static border that
  // never animates (see the card className below).
  const effectsEnabled = useEffectsEnabled()
  const planToggleDisabled = disabled || !planAvailable
  const goalToggleDisabled = disabled || !goalAvailable || goalActive
  const planModeOn = planAvailable && (planModeOverride ?? planModeActive)
  const goalModeOn = goalAvailable && (goalRequested || goalActive)
  const showGoalChip = goalAvailable && (goalRequested || goalActive)
  const mention = useMentionInput({
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
  const canQueueWhileStreaming = streaming && queueWhenStreaming
  const attachmentBusy = attachmentDraft.busy
  const hasNonTextDraftContent =
    attachmentDraft.files.length > 0 || attachmentDraft.uploaded.length > 0 || contexts.length > 0
  const hasSendableDraft = (messageEmpty: boolean) => !messageEmpty || hasNonTextDraftContent
  const hasDraftContent = hasSendableDraft(mention.isEmpty)
  const submitDisabled = !hasDraftContent || disabled || attachmentBusy || (streaming && !canQueueWhileStreaming)
  const showStopButton = streaming && onStop && (!queueWhenStreaming || !hasDraftContent)

  // autoFocus lands before React's focus listeners attach; sync the ring state.
  useEffect(() => {
    if (document.activeElement === mention.textareaRef.current) setFocused(true)
  }, [mention.textareaRef])

  useEffect(() => {
    if (!planAvailable) setPlanModeOverride(null)
  }, [planAvailable])

  useEffect(() => {
    setPlanModeOverride((override) => (override === planModeActive ? null : override))
  }, [planModeActive])

  useEffect(() => {
    if (!goalAvailable || goalActive) setGoalRequested(false)
  }, [goalActive, goalAvailable])

  const { dropTargetRef, dragging: draggingFiles } = useFileDropTarget<HTMLDivElement>({
    disabled,
    onDrop: attachmentDraft.addFiles,
  })

  const onPasteCapture = (event: ClipboardEvent<HTMLDivElement>) => {
    const files = clipboardFiles(event.clipboardData)
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

  const toggleGoalRequested = () => {
    if (goalToggleDisabled) return
    setGoalRequested((value) => !value)
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
    mention.reset()
    attachmentDraft.clearAttachments()
    onClearContexts?.()
    setGoalRequested(false)
  }

  const submit = async () => {
    // Tokens expand on the way out: tagged paths become absolute, skill
    // references pass through for the agent's skill catalog to resolve.
    const trimmed = mention.value().trim()
    if (
      !hasSendableDraft(trimmed === '') ||
      disabled ||
      attachmentBusy ||
      (streaming && !canQueueWhileStreaming)
    ) {
      return
    }
    try {
      await onSend(trimmed, {
        planRequested: planModeOn,
        goalRequested: goalModeOn,
        files: attachmentDraft.files,
        attachments: attachmentDraft.uploaded,
        ...(contexts.length > 0 ? { contexts } : {}),
      })
      if (clearOnSend) clearDraft()
    } catch {
      // Sender owns user-facing failure state; keep the draft intact.
    }
  }

  return (
    <div
      ref={dropTargetRef}
      className="relative"
      onPasteCapture={onPasteCapture}
      onFocusCapture={() => setFocused(true)}
      onBlurCapture={(e) => {
        if (!e.currentTarget.contains(e.relatedTarget as Node | null)) setFocused(false)
      }}
    >
      <FileDropOverlay visible={draggingFiles} />
      <MentionSuggestions mention={mention} placement="above" />
      <AnimatePresence>
        {focused && effectsEnabled ? (
          <motion.div
            key="ring"
            aria-hidden
            className="pointer-events-none absolute -inset-[2px]"
            initial={{ opacity: 0 }}
            animate={{
              opacity: 1,
              ...(reducedMotion ? {} : { '--ring-angle': ['0deg', '360deg'] }),
            }}
            exit={{ opacity: 0 }}
            transition={{
              opacity: { duration: 0.25, ease: 'easeOut' },
              '--ring-angle': { duration: 2.6, ease: 'linear', repeat: Infinity },
            }}
          >
            {/* glow trailing the comet, bleeding softly outside the card */}
            <div
              className="absolute -inset-[4px] rounded-[18px] opacity-50 blur-[10px]"
              style={{ background: RAINBOW_BEAM }}
            />
            {/* the comet itself; the card's opaque surface covers the center */}
            <div className="absolute inset-0 rounded-[14px]" style={{ background: RAINBOW_BEAM }} />
          </motion.div>
        ) : null}
      </AnimatePresence>

      {/* borderless card, agent-council style: the surface tone IS the card.
          The whole card is a click target for the textarea. */}
      <div
        className={`relative flex cursor-text flex-col gap-1.5 rounded-[12px] p-2.5 transition-shadow ${
          translucent ? 'bg-surface/85 backdrop-blur-[2px]' : 'bg-surface'
        } ${
          !effectsEnabled
            ? focused
              ? 'ring-2 ring-primary'
              : 'ring-1 ring-border'
            : ''
        } ${draggingFiles ? 'shadow-[0_0_0_1px_var(--color-primary),0_10px_35px_rgba(0,0,0,0.16)]' : ''}`}
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
        <ComposerAttachmentList attachments={attachmentDraft.attachments} onRemove={attachmentDraft.removeAttachment} />
        <MentionTextarea
          mention={mention}
          placeholder={placeholder}
          disabled={disabled}
          autoFocus={autoFocus}
          onKeyDown={(e) => {
            if (e.key === 'Enter' && !e.shiftKey) {
              e.preventDefault()
              void submit()
            }
          }}
        />
        <div className="flex items-center justify-between gap-2.5 max-sm:items-end">
          {/* Phone: the new-thread controls (agent, model, project, worktree)
              outgrow one row, so let them wrap and keep send pinned bottom-right. */}
          <div className="flex min-w-0 items-center gap-1.5 max-sm:flex-1 max-sm:flex-wrap">
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
                <PlanMenuToggle
                  checked={planModeOn}
                  disabled={disabled}
                  onToggle={togglePlanMode}
                />
              ) : null}
              {goalControlVisible ? (
                goalAvailable ? (
                  <GoalMenuToggle
                    checked={goalActive || goalRequested}
                    disabled={goalToggleDisabled}
                    onToggle={toggleGoalRequested}
                  />
                ) : (
                  <GoalUnsupportedRow />
                )
              ) : null}
            </Popover>
            {leftSlot}
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
                  active={goalActive}
                  requested={goalRequested}
                  disabled={disabled}
                  onRemove={() => setGoalRequested(false)}
                />
              ) : null}
            </AnimatePresence>
          </div>
          <div className="flex shrink-0 items-center gap-1.5">
            {onVoice ? (
              <IconButton
                variant="ghost"
                size="lg"
                aria-label="Voice mode"
                title="Voice mode"
                disabled={streaming || disabled}
                onClick={onVoice}
              >
                <AudioLines size={16} />
              </IconButton>
            ) : null}
            {showStopButton ? (
              <IconButton
                variant="primary"
                size="lg"
                aria-label="Stop response"
                title="Stop response"
                onClick={onStop}
              >
                <Square size={13} fill="currentColor" strokeWidth={0} />
              </IconButton>
            ) : (
              <IconButton
                variant="primary"
                size="lg"
                aria-label={streaming ? 'Queue message' : 'Send message'}
                title={streaming ? 'Queue message' : 'Send message'}
                disabled={submitDisabled}
                onClick={() => void submit()}
              >
                <ArrowUp size={18} />
              </IconButton>
            )}
          </div>
        </div>
      </div>
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
  goalActive,
  queuedPrompts = [],
  steerDisabled,
  draftStorageKey,
  fileRoot,
  contexts,
  onSend,
  onStop,
  onVoice,
  onUploadAttachment,
  onRemoveContext,
  onClearContexts,
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
  goalActive?: boolean
  queuedPrompts?: QueuedMessage[]
  steerDisabled?: boolean
  draftStorageKey?: string
  /** directory the @-mention file picker indexes; undefined disables files */
  fileRoot?: string
  contexts?: ComposerContext[]
  onSend: SendMessageHandler
  onStop: () => void
  onVoice?: () => void
  onUploadAttachment?: (file: File) => Promise<Attachment>
  onRemoveContext?: (id: string) => void
  onClearContexts?: () => void
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
        goalActive={goalActive}
        queueWhenStreaming
        draftStorageKey={draftStorageKey}
        draftStorage="local"
        fileRoot={fileRoot}
        contexts={contexts}
        onSend={onSend}
        onStop={onStop}
        onVoice={onVoice}
        onUploadAttachment={onUploadAttachment}
        onRemoveContext={onRemoveContext}
        onClearContexts={onClearContexts}
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
