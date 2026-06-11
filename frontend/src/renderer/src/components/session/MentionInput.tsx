import { useQuery } from '@tanstack/react-query'
import { AnimatePresence } from 'motion/react'
import {
  type ChangeEvent,
  type KeyboardEvent,
  type SyntheticEvent,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react'
import { workspaceFilesQuery } from '@/lib/api/sessions'
import { skillsQuery } from '@/lib/api/skills'
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
import { decodeMentions, encodeMention } from './mentions'

// Result cap for the $/@ popups, mirroring Codex's file-search page size.
const MAX_SUGGESTIONS = 20

// Shared by the textarea and its token-highlight mirror — any drift in these
// box/text metrics would misalign the caret with the painted glyphs.
const TEXT_CLASSES = 'px-2 pt-1.5 pb-0.5 text-sm leading-relaxed'

// fromWire parses message text with encoded mentions back into the editor
// model: plain display text plus the token map keyed by display.
function fromWire(wire: string): { text: string; tokens: Map<string, InlineToken> } {
  const tokens = new Map<string, InlineToken>()
  let text = ''
  for (const segment of decodeMentions(wire)) {
    text += segment.text
    if (segment.mention) {
      tokens.set(segment.text, {
        trigger: segment.mention.sigil,
        display: segment.text,
        expansion: encodeMention(segment.mention.sigil, segment.mention.name, segment.mention.target),
      })
    }
  }
  return { text, tokens }
}

export type MentionInput = ReturnType<typeof useMentionInput>

// The editing model behind a mention-capable textarea: inline $skill / @path
// tokens whose display text lives in `text`, keyed here by that display, with
// the $/@ trigger menu derived from text + caret. `dismissedAt` remembers an
// Escape'd trigger position so the menu stays closed until the trigger changes.
export function useMentionInput({
  fileRoot,
  disabled,
  maxHeight = 200,
  initialValue = '',
  onValueChange,
}: {
  /** server-side directory the @-mention picker indexes (a project path, a
      session cwd, or '' for the workspace root). undefined disables @ */
  fileRoot?: string
  disabled?: boolean
  /** auto-grow cap for the textarea, in px */
  maxHeight?: number
  /** initial value in wire form (text with encoded mentions) */
  initialValue?: string
  /** reports the wire-form value after every edit */
  onValueChange?: (value: string) => void
}) {
  const [text, setText] = useState(() => fromWire(initialValue).text)
  const [tokens, setTokens] = useState(() => fromWire(initialValue).tokens)
  const [caret, setCaret] = useState(0)
  const [dismissedAt, setDismissedAt] = useState<number | null>(null)
  const [activeIndex, setActiveIndex] = useState(0)
  // Fetches prewarm and the menu only opens while the textarea is focused;
  // MentionTextarea wires the focus/blur events.
  const [focused, setFocused] = useState(false)
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  const mirrorRef = useRef<HTMLDivElement>(null)
  const composingRef = useRef(false)

  // autoFocus lands before React's focus listeners attach; sync the initial state.
  useEffect(() => {
    if (document.activeElement === textareaRef.current) setFocused(true)
  }, [])

  // Open-ness is derived from text + caret, never stored: moving the caret
  // away, typing whitespace, or deleting the trigger all close the menu for
  // free.
  const trigger = useMemo(() => findActiveTrigger(text, caret), [text, caret])
  const menuTrigger = trigger && trigger.start !== dismissedAt && !disabled ? trigger : null
  const skills = useQuery({ ...skillsQuery, enabled: focused })
  const fileIndex = useQuery({
    ...workspaceFilesQuery(fileRoot ?? ''),
    enabled: fileRoot !== undefined && focused,
  })
  const skillMentionStart = focused && menuTrigger?.trigger === '$' ? menuTrigger.start : null

  const { refetch: refetchSkills } = skills
  useEffect(() => {
    if (skillMentionStart === null) return
    void refetchSkills()
  }, [skillMentionStart, refetchSkills])

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

  // Reports edits only — the mount pass over initialValue stays silent.
  const onValueChangeRef = useRef(onValueChange)
  onValueChangeRef.current = onValueChange
  const editedRef = useRef(false)
  useEffect(() => {
    if (!editedRef.current) {
      editedRef.current = true
      return
    }
    onValueChangeRef.current?.(expandTokens(text, tokens))
  }, [text, tokens])

  const autoGrow = useCallback(
    (el?: HTMLTextAreaElement | null) => {
      const target = el ?? textareaRef.current
      if (!target) return
      target.style.height = 'auto'
      target.style.height = `${Math.min(target.scrollHeight, maxHeight)}px`
    },
    [maxHeight],
  )

  // Size to any prefilled value on mount; rows={1} would collapse it.
  useEffect(() => {
    autoGrow()
  }, [autoGrow])

  // Programmatic edits don't fire onChange; restore the caret and replay the
  // auto-grow after React commits the new value.
  const placeCaret = (pos: number) => {
    requestAnimationFrame(() => {
      const el = textareaRef.current
      if (!el) return
      el.focus()
      el.setSelectionRange(pos, pos)
      autoGrow(el)
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

  const reset = () => {
    setText('')
    setTokens(new Map())
    setCaret(0)
    setDismissedAt(null)
    const el = textareaRef.current
    if (el) {
      el.style.height = 'auto'
      el.focus()
    }
  }

  const onChange = (e: ChangeEvent<HTMLTextAreaElement>) => {
    const next = e.target.value
    setText(next)
    setCaret(e.target.selectionStart ?? next.length)
    setTokens((prev) => pruneTokens(prev, next))
    // an Escape'd trigger stays dismissed only while it's still the same
    // trigger; editing elsewhere re-arms the menu
    setDismissedAt((dismissed) => {
      if (dismissed === null) return null
      const active = findActiveTrigger(next, e.target.selectionStart ?? next.length)
      return active && active.start === dismissed ? dismissed : null
    })
    autoGrow(e.target)
  }

  const onSelect = (e: SyntheticEvent<HTMLTextAreaElement>) => {
    const el = e.currentTarget
    setCaret(el.selectionStart === el.selectionEnd ? (el.selectionStart ?? 0) : -1)
  }

  const onScroll = (e: SyntheticEvent<HTMLTextAreaElement>) => {
    if (mirrorRef.current) mirrorRef.current.scrollTop = e.currentTarget.scrollTop
  }

  const onCompositionStart = () => {
    composingRef.current = true
  }

  const onCompositionEnd = () => {
    composingRef.current = false
  }

  // Returns true when the key was consumed by the mention machinery (or must
  // be left to the IME mid-composition); callers skip their own handling then.
  const onKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>): boolean => {
    if (composingRef.current || e.nativeEvent.isComposing) return true
    if (menuOpen) {
      if (e.key === 'ArrowDown') {
        e.preventDefault()
        setActiveIndex((index) => Math.min(index + 1, flatItems.length - 1))
        return true
      }
      if (e.key === 'ArrowUp') {
        e.preventDefault()
        setActiveIndex((index) => Math.max(index - 1, 0))
        return true
      }
      if (e.key === 'Enter' || e.key === 'Tab') {
        e.preventDefault()
        const item = flatItems[Math.min(activeIndex, flatItems.length - 1)]
        if (item) selectItem(item)
        return true
      }
      if (e.key === 'Escape') {
        e.preventDefault()
        e.stopPropagation()
        if (menuTrigger) setDismissedAt(menuTrigger.start)
        return true
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
          return true
        }
      }
    }
    return false
  }

  return {
    text,
    isEmpty: text.trim() === '',
    segments,
    sections,
    menuOpen,
    activeIndex,
    setActiveIndex,
    selectItem,
    textareaRef,
    mirrorRef,
    maxHeight,
    /** the current value in wire form (encoded mentions) */
    value: () => expandTokens(text, tokens),
    reset,
    onFocus: () => setFocused(true),
    onBlur: () => setFocused(false),
    onChange,
    onSelect,
    onScroll,
    onCompositionStart,
    onCompositionEnd,
    onKeyDown,
  }
}

// The floating $/@ menu for a mention field. Renders into the nearest
// positioned ancestor — callers wrap their field in a `relative` container.
export function MentionSuggestions({
  mention,
  placement,
}: {
  mention: MentionInput
  placement: 'above' | 'below'
}) {
  return (
    <AnimatePresence>
      {mention.menuOpen ? (
        <div
          key="suggestions"
          className={`absolute inset-x-0 z-30 ${
            placement === 'above' ? 'bottom-full mb-2' : 'top-full mt-2'
          }`}
        >
          <ComposerSuggestions
            sections={mention.sections}
            activeIndex={mention.activeIndex}
            onHover={mention.setActiveIndex}
            onSelect={mention.selectItem}
          />
        </div>
      ) : null}
    </AnimatePresence>
  )
}

// The painted pair behind every mention-capable field: a mirror painting the
// text and token highlights, with the real textarea above it keeping
// transparent glyphs (the caret via caret-ink). The textarea's selection
// highlight must stay translucent for the mirror glyphs to read through it.
export function MentionTextarea({
  mention,
  placeholder,
  disabled,
  autoFocus,
  minHeightClass = 'min-h-[30px]',
  onKeyDown,
}: {
  mention: MentionInput
  placeholder?: string
  disabled?: boolean
  autoFocus?: boolean
  minHeightClass?: string
  /** runs only when the mention machinery didn't consume the key */
  onKeyDown?: (e: KeyboardEvent<HTMLTextAreaElement>) => void
}) {
  return (
    <div className="relative">
      <div
        ref={mention.mirrorRef}
        aria-hidden
        className={`pointer-events-none absolute inset-0 overflow-hidden whitespace-pre-wrap [overflow-wrap:break-word] ${TEXT_CLASSES} text-ink`}
      >
        {mention.segments.map((segment, index) =>
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
        ref={mention.textareaRef}
        value={mention.text}
        rows={1}
        autoFocus={autoFocus}
        disabled={disabled}
        placeholder={placeholder}
        aria-autocomplete="list"
        aria-expanded={mention.menuOpen}
        // No spellcheck: squiggles under skill/path tokens read as errors.
        spellCheck={false}
        className={`composer-input relative z-[1] w-full resize-none bg-transparent ${minHeightClass} ${TEXT_CLASSES} text-transparent caret-ink select-text placeholder:text-ink-3 disabled:cursor-default`}
        style={{ maxHeight: mention.maxHeight }}
        onFocus={mention.onFocus}
        onBlur={mention.onBlur}
        onScroll={mention.onScroll}
        onCompositionStart={mention.onCompositionStart}
        onCompositionEnd={mention.onCompositionEnd}
        onSelect={mention.onSelect}
        onChange={mention.onChange}
        onKeyDown={(e) => {
          if (mention.onKeyDown(e)) return
          onKeyDown?.(e)
        }}
      />
    </div>
  )
}
