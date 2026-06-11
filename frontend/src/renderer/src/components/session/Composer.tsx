import { useQuery } from '@tanstack/react-query'
import { ArrowUp, AudioLines, Check, FileText, ListChecks, LoaderCircle, Paperclip, Plus, Square, X } from 'lucide-react'
import { AnimatePresence, motion, useReducedMotion } from 'motion/react'
import { type ReactNode, useEffect, useMemo, useRef, useState } from 'react'
import { IconButton } from '@/components/ui/IconButton'
import type { SendMessageOptions } from '@/lib/sendMessage'
import { skillsQuery } from '@/lib/api/skills'
import { workspaceFilesQuery } from '@/lib/api/sessions'
import { MenuRow, Popover } from '@/components/ui/Popover'
import { ComposerSuggestions, type SuggestionItem, type SuggestionSection } from './ComposerSuggestions'
import {
  expandTokens,
  findActiveTrigger,
  pruneTokens,
  segmentValue,
  tokenEndingAt,
  type InlineToken,
} from './composerTokens'
import { fuzzyMatch } from './fuzzy'
import { encodeMention } from './mentions'
import { QueuedPromptList } from './QueuedPromptList'

// A rainbow comet (~100° arc fading in and out of transparency) that orbits
// the card while focused; the rest of the perimeter stays a quiet track.
// Shared with the music bubbles' now-playing ring.
export const RAINBOW_BEAM =
  'conic-gradient(from var(--ring-angle, 0deg), transparent 0deg 250deg, var(--color-rainbow-1) 278deg, var(--color-rainbow-2) 296deg, var(--color-rainbow-3) 312deg, var(--color-rainbow-4) 326deg, var(--color-rainbow-5) 340deg, transparent 352deg 360deg)'

// Shared by the textarea and its token-highlight mirror — any drift in these
// box/text metrics would misalign the caret with the painted glyphs.
const COMPOSER_TEXT_CLASSES = 'px-2 pt-1.5 pb-0.5 text-sm leading-relaxed'

// Result cap for the $/@ popups, mirroring Codex's file-search page size.
const MAX_SUGGESTIONS = 20

function formatFileSize(size: number): string {
  if (size < 1024) return `${size} B`
  if (size < 1024 * 1024) return `${Math.round(size / 1024)} KB`
  return `${(size / (1024 * 1024)).toFixed(1)} MB`
}

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
      className={`flex h-7 w-full items-center gap-2 rounded-full px-2.5 text-left text-[13px] transition-colors duration-150 hover:bg-surface-2 disabled:cursor-default disabled:opacity-50 ${
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
  placeholder = 'Message your assistant…',
  disabled = false,
  planAvailable = false,
  queueWhenStreaming = false,
  translucent = false,
  leftSlot,
  fileRoot,
  onSend,
  onStop,
  onVoice,
}: {
  streaming: boolean
  autoFocus?: boolean
  placeholder?: string
  disabled?: boolean
  planAvailable?: boolean
  queueWhenStreaming?: boolean
  /** let a backdrop (e.g. the welcome particle field) read through the card */
  translucent?: boolean
  /** leading toolbar content (e.g. the new-thread runtime/directory pickers) */
  leftSlot?: ReactNode
  /** server-side directory the @-mention picker indexes (a session cwd, or a
      workspace-relative pick; '' is the workspace root). undefined disables @ */
  fileRoot?: string
  onSend: (text: string, options?: SendMessageOptions) => void
  onStop?: () => void
  onVoice?: () => void
}) {
  const [text, setText] = useState('')
  const [files, setFiles] = useState<File[]>([])
  const [focused, setFocused] = useState(false)
  const [draggingFiles, setDraggingFiles] = useState(false)
  const [optionsOpen, setOptionsOpen] = useState(false)
  const [planRequested, setPlanRequested] = useState(false)
  // Inline $skill / @path tokens: display text lives in `text`, keyed here by
  // that display. `dismissedAt` remembers an Escape'd trigger position so the
  // menu stays closed until the trigger changes.
  const [tokens, setTokens] = useState<Map<string, InlineToken>>(new Map())
  const [caret, setCaret] = useState(0)
  const [dismissedAt, setDismissedAt] = useState<number | null>(null)
  const [activeIndex, setActiveIndex] = useState(0)
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  const mirrorRef = useRef<HTMLDivElement>(null)
  const composingRef = useRef(false)
  const fileInputRef = useRef<HTMLInputElement>(null)
  const reducedMotion = useReducedMotion()

  // Open-ness is derived from text + caret, never stored: moving the caret
  // away, typing whitespace, or deleting the trigger all close the menu for
  // free. Fetches prewarm on focus so the menu opens populated.
  const trigger = useMemo(() => findActiveTrigger(text, caret), [text, caret])
  const menuTrigger = trigger && trigger.start !== dismissedAt && !disabled ? trigger : null
  const skills = useQuery({ ...skillsQuery, enabled: focused })
  const fileIndex = useQuery({
    ...workspaceFilesQuery(fileRoot ?? ''),
    enabled: fileRoot !== undefined && focused,
  })

  const sections = useMemo<SuggestionSection[]>(() => {
    if (!menuTrigger) return []
    const query = menuTrigger.query
    if (menuTrigger.trigger === '$') {
      const items = (skills.data ?? [])
        .flatMap((skill) => {
          const match = fuzzyMatch(query, skill.name)
          return match ? [{ skill, match }] : []
        })
        .sort(
          (a, b) => b.match.score - a.match.score || a.skill.name.localeCompare(b.skill.name),
        )
        .slice(0, MAX_SUGGESTIONS)
        .map(({ skill, match }) => ({
          kind: 'skill' as const,
          label: skill.name,
          detail: skill.description,
          indices: match.indices,
          insert: `$${skill.name}`,
          // Sent as a linked mention: self-describing for the agent (name +
          // SKILL.md location inline) and decoded back to a pill in the
          // transcript.
          expansion: encodeMention('$', skill.name, skill.path),
        }))
      return items.length > 0 ? [{ title: 'Skills', items }] : []
    }
    const index = fileIndex.data
    if (!index || index.root === '') return []
    // Fuzzy score over full relative paths (Codex's pattern): a query naming
    // a directory scores the directory and its children identically, so the
    // tie-breaks — directory first, then shallow before deep — surface the
    // folder followed by the files inside it.
    const items = index.entries
      .flatMap((entry) => {
        const match = fuzzyMatch(query, entry.path)
        return match ? [{ entry, match }] : []
      })
      .sort(
        (a, b) =>
          b.match.score - a.match.score ||
          Number(b.entry.dir) - Number(a.entry.dir) ||
          a.entry.path.length - b.entry.path.length ||
          a.entry.path.localeCompare(b.entry.path),
      )
      .slice(0, MAX_SUGGESTIONS)
      .map(({ entry, match }) => ({
        kind: entry.dir ? ('dir' as const) : ('file' as const),
        label: entry.path,
        indices: match.indices,
        insert: `@${entry.path}`,
        expansion: encodeMention('@', entry.path, `${index.root}/${entry.path}`),
      }))
    return items.length > 0 ? [{ title: 'Files', items }] : []
  }, [menuTrigger, skills.data, fileIndex.data])

  const flatItems = useMemo(() => sections.flatMap((section) => section.items), [sections])
  const menuOpen = menuTrigger !== null && flatItems.length > 0 && focused

  useEffect(() => {
    setActiveIndex(0)
  }, [menuTrigger?.trigger, menuTrigger?.start, menuTrigger?.query])

  const segments = useMemo(() => segmentValue(text, tokens), [text, tokens])

  // Programmatic edits don't fire onChange; restore the caret and replay the
  // auto-grow after React commits the new value.
  const placeCaret = (pos: number) => {
    requestAnimationFrame(() => {
      const el = textareaRef.current
      if (!el) return
      el.focus()
      el.setSelectionRange(pos, pos)
      el.style.height = 'auto'
      el.style.height = `${Math.min(el.scrollHeight, 200)}px`
    })
  }

  const selectItem = (item: SuggestionItem) => {
    if (!menuTrigger) return
    // The trailing space ends the trigger (closing the menu) and gives the
    // atomic backspace an obvious feel: one press eats the space, the next
    // eats the whole token.
    const next = `${text.slice(0, menuTrigger.start)}${item.insert} ${text.slice(caret)}`
    setTokens((prev) =>
      new Map(prev).set(item.insert, {
        trigger: menuTrigger.trigger,
        display: item.insert,
        expansion: item.expansion,
      }),
    )
    setText(next)
    const pos = menuTrigger.start + item.insert.length + 1
    setCaret(pos)
    placeCaret(pos)
  }

  // autoFocus lands before React's focus listeners attach; sync the initial state.
  useEffect(() => {
    if (document.activeElement === textareaRef.current) setFocused(true)
  }, [])

  useEffect(() => {
    if (!planAvailable) setPlanRequested(false)
  }, [planAvailable])

  const addFiles = (next: File[]) => {
    if (disabled || streaming || next.length === 0) return
    setFiles((current) => [...current, ...next])
  }

  const togglePlanRequested = () => {
    if (disabled || streaming || !planAvailable) return
    setPlanRequested((value) => !value)
  }

  useEffect(() => {
    if (disabled || streaming || !planAvailable) return
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
      setPlanRequested((value) => !value)
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [disabled, planAvailable, streaming])

  const submit = () => {
    // Tokens expand on the way out: tagged paths become absolute, skill
    // references pass through for the agent's skill catalog to resolve.
    const trimmed = expandTokens(text, tokens).trim()
    if (!trimmed || disabled || (streaming && (!queueWhenStreaming || files.length > 0))) return
    onSend(trimmed, {
      planRequested: !streaming && planAvailable && planRequested,
      files: streaming ? [] : files,
    })
    setText('')
    setTokens(new Map())
    setCaret(0)
    setDismissedAt(null)
    setFiles([])
    setPlanRequested(false)
    const el = textareaRef.current
    if (el) {
      el.style.height = 'auto'
      el.focus()
    }
  }

  return (
    <div
      className="relative"
      onFocusCapture={() => setFocused(true)}
      onBlurCapture={(e) => {
        if (!e.currentTarget.contains(e.relatedTarget as Node | null)) setFocused(false)
      }}
      onDragEnter={(e) => {
        if (!Array.from(e.dataTransfer.types).includes('Files')) return
        e.preventDefault()
        if (!disabled && !streaming) setDraggingFiles(true)
      }}
      onDragOver={(e) => {
        if (!Array.from(e.dataTransfer.types).includes('Files')) return
        e.preventDefault()
      }}
      onDragLeave={(e) => {
        if (!e.currentTarget.contains(e.relatedTarget as Node | null)) setDraggingFiles(false)
      }}
      onDrop={(e) => {
        if (!Array.from(e.dataTransfer.types).includes('Files')) return
        e.preventDefault()
        setDraggingFiles(false)
        addFiles(Array.from(e.dataTransfer.files))
      }}
    >
      <AnimatePresence>
        {menuOpen ? (
          <div key="suggestions" className="absolute inset-x-0 bottom-full z-30 mb-2">
            <ComposerSuggestions
              sections={sections}
              activeIndex={activeIndex}
              onHover={setActiveIndex}
              onSelect={selectItem}
            />
          </div>
        ) : null}
      </AnimatePresence>
      <AnimatePresence>
        {focused ? (
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
        } ${draggingFiles ? 'shadow-[0_0_0_1px_var(--color-primary),0_10px_35px_rgba(0,0,0,0.16)]' : ''}`}
        onClick={(e) => {
          if ((e.target as HTMLElement).closest('button, textarea, input')) return
          textareaRef.current?.focus()
        }}
      >
        <input
          ref={fileInputRef}
          type="file"
          multiple
          className="hidden"
          disabled={disabled || streaming}
          onChange={(e) => {
            addFiles(Array.from(e.currentTarget.files ?? []))
            e.currentTarget.value = ''
          }}
        />
        {files.length > 0 ? (
          <div className="flex flex-wrap gap-1 px-1.5 pt-0.5">
            {files.map((file, index) => (
              <div
                key={`${file.name}-${file.size}-${index}`}
                className="flex max-w-full items-center gap-1.5 rounded-full bg-bg px-2.5 py-1 text-xs text-ink-2"
              >
                <FileText size={13} className="shrink-0 text-primary" />
                <span className="max-w-[220px] truncate text-ink">{file.name}</span>
                <span className="shrink-0 text-ink-3">{formatFileSize(file.size)}</span>
                <button
                  type="button"
                  className="ml-0.5 rounded-full p-0.5 text-ink-3 transition-colors hover:bg-surface hover:text-ink"
                  aria-label={`Remove ${file.name}`}
                  onClick={() => setFiles((current) => current.filter((_, i) => i !== index))}
                >
                  <X size={12} />
                </button>
              </div>
            ))}
          </div>
        ) : null}
        <div className="relative">
          {/* Mirror painting the text and token highlights: identical
              box/text metrics to the textarea, whose own glyphs stay
              transparent (the caret via caret-ink). The textarea sits above,
              so its selection highlight must be translucent for the mirror
              glyphs to read through it. */}
          <div
            ref={mirrorRef}
            aria-hidden
            className={`pointer-events-none absolute inset-0 overflow-hidden whitespace-pre-wrap [overflow-wrap:break-word] ${COMPOSER_TEXT_CLASSES} text-ink`}
          >
            {segments.map((segment, index) =>
              segment.token ? (
                <span key={index} className="rounded-[4px] bg-primary-soft text-primary-strong">
                  {segment.text}
                </span>
              ) : (
                <span key={index}>{segment.text}</span>
              ),
            )}
            {/* keeps a trailing newline's empty line box in the mirror */}
            {'\u200b'}
          </div>
          <textarea
            ref={textareaRef}
            value={text}
            rows={1}
            autoFocus={autoFocus}
            disabled={disabled}
            placeholder={placeholder}
            aria-autocomplete="list"
            aria-expanded={menuOpen}
            className={`relative z-[1] max-h-[200px] min-h-[30px] w-full resize-none bg-transparent ${COMPOSER_TEXT_CLASSES} text-transparent caret-ink select-text selection:bg-primary/25 placeholder:text-ink-3 disabled:cursor-default`}
            onScroll={(e) => {
              if (mirrorRef.current) mirrorRef.current.scrollTop = e.currentTarget.scrollTop
            }}
            onCompositionStart={() => {
              composingRef.current = true
            }}
            onCompositionEnd={() => {
              composingRef.current = false
            }}
            onSelect={(e) => {
              const el = e.currentTarget
              setCaret(el.selectionStart === el.selectionEnd ? (el.selectionStart ?? 0) : -1)
            }}
            onChange={(e) => {
              const next = e.target.value
              setText(next)
              setCaret(e.target.selectionStart ?? next.length)
              setTokens((prev) => pruneTokens(prev, next))
              // an Escape'd trigger stays dismissed only while it's still the
              // same trigger; editing elsewhere re-arms the menu
              setDismissedAt((dismissed) => {
                if (dismissed === null) return null
                const active = findActiveTrigger(next, e.target.selectionStart ?? next.length)
                return active && active.start === dismissed ? dismissed : null
              })
              e.target.style.height = 'auto'
              e.target.style.height = `${Math.min(e.target.scrollHeight, 200)}px`
            }}
            onKeyDown={(e) => {
              if (composingRef.current || e.nativeEvent.isComposing) return
              if (menuOpen) {
                if (e.key === 'ArrowDown') {
                  e.preventDefault()
                  setActiveIndex((index) => Math.min(index + 1, flatItems.length - 1))
                  return
                }
                if (e.key === 'ArrowUp') {
                  e.preventDefault()
                  setActiveIndex((index) => Math.max(index - 1, 0))
                  return
                }
                if (e.key === 'Enter' || e.key === 'Tab') {
                  e.preventDefault()
                  const item = flatItems[Math.min(activeIndex, flatItems.length - 1)]
                  if (item) selectItem(item)
                  return
                }
                if (e.key === 'Escape') {
                  e.preventDefault()
                  e.stopPropagation()
                  if (menuTrigger) setDismissedAt(menuTrigger.start)
                  return
                }
              }
              if (e.key === 'Backspace' && !e.metaKey && !e.ctrlKey && !e.altKey) {
                const el = e.currentTarget
                if (el.selectionStart === el.selectionEnd) {
                  const hit = tokenEndingAt(text, el.selectionStart, tokens)
                  if (hit) {
                    e.preventDefault()
                    const next = text.slice(0, hit.start) + text.slice(el.selectionStart)
                    setTokens((prev) => pruneTokens(prev, next))
                    setText(next)
                    setCaret(hit.start)
                    placeCaret(hit.start)
                    return
                  }
                }
              }
              if (e.key === 'Enter' && !e.shiftKey) {
                e.preventDefault()
                submit()
              }
            }}
          />
        </div>
        <div className="flex items-center justify-between gap-2.5">
          <div className="flex min-w-0 items-center gap-1.5">
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
                  disabled={streaming || disabled}
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
              <MenuRow
                onClick={() => {
                  setOptionsOpen(false)
                  fileInputRef.current?.click()
                }}
              >
                <span className="flex items-center gap-2">
                  <Paperclip size={13} />
                  Attach files
                </span>
              </MenuRow>
              {planAvailable ? (
                <PlanMenuToggle checked={planRequested} disabled={streaming || disabled} onToggle={togglePlanRequested} />
              ) : null}
            </Popover>
            {leftSlot}
            <AnimatePresence initial={false}>
              {planRequested ? (
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
                    disabled={streaming || disabled}
                    className="grid"
                    onClick={() => setPlanRequested(false)}
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
            {streaming && onStop && (!text.trim() || !queueWhenStreaming) ? (
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
                disabled={!text.trim() || disabled || (streaming && (!queueWhenStreaming || files.length > 0))}
                onClick={submit}
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

// Bottom dock for the session view: content scrolls beneath a fade into the
// page background; only the card itself receives pointer events.
export function Composer({
  streaming,
  disabled,
  placeholder,
  planAvailable,
  queuedPrompts = [],
  steerDisabled,
  fileRoot,
  onSend,
  onStop,
  onVoice,
  onSteerQueuedPrompt,
  onDeleteQueuedPrompt,
  onEditQueuedPrompt,
  onMoveQueuedPrompt,
}: {
  streaming: boolean
  disabled?: boolean
  placeholder?: string
  planAvailable?: boolean
  queuedPrompts?: string[]
  steerDisabled?: boolean
  /** directory the @-mention picker indexes; undefined disables @ */
  fileRoot?: string
  onSend: (text: string, options?: SendMessageOptions) => void
  onStop: () => void
  onVoice?: () => void
  onSteerQueuedPrompt?: (index: number) => void
  onDeleteQueuedPrompt?: (index: number) => void
  onEditQueuedPrompt?: (index: number, text: string) => void
  onMoveQueuedPrompt?: (from: number, to: number) => void
}) {
  return (
    <div className="pointer-events-none absolute inset-x-0 bottom-0 bg-gradient-to-b from-transparent to-bg to-45% px-10 pt-6 pb-5">
      <motion.div
        className="pointer-events-auto mx-auto max-w-[640px]"
        initial={{ y: 12, opacity: 0 }}
        animate={{ y: 0, opacity: 1 }}
        transition={{ type: 'spring', stiffness: 380, damping: 32 }}
      >
        <AnimatePresence initial={false}>
          {queuedPrompts.length > 0 &&
          onSteerQueuedPrompt &&
          onDeleteQueuedPrompt &&
          onEditQueuedPrompt &&
          onMoveQueuedPrompt ? (
            <QueuedPromptList
              prompts={queuedPrompts}
              steerDisabled={steerDisabled}
              onSteer={onSteerQueuedPrompt}
              onDelete={onDeleteQueuedPrompt}
              onEdit={onEditQueuedPrompt}
              onMove={onMoveQueuedPrompt}
            />
          ) : null}
        </AnimatePresence>
        <ComposerCard
          streaming={streaming}
          disabled={disabled}
          placeholder={placeholder}
          planAvailable={planAvailable}
          queueWhenStreaming
          fileRoot={fileRoot}
          onSend={onSend}
          onStop={onStop}
          onVoice={onVoice}
        />
      </motion.div>
    </div>
  )
}

export function PlanDecisionDock({
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
    <div className="pointer-events-none absolute inset-x-0 bottom-0 bg-gradient-to-b from-transparent to-bg to-45% px-10 pt-6 pb-5">
      <motion.div
        className="pointer-events-auto mx-auto max-w-[640px]"
        initial={{ y: 12, opacity: 0 }}
        animate={{ y: 0, opacity: 1 }}
        transition={{ type: 'spring', stiffness: 380, damping: 32 }}
      >
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
              ) : (
                <Check size={15} className="shrink-0 text-primary" />
              )}
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
      </motion.div>
    </div>
  )
}
