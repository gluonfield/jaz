import { motion, type Transition } from 'motion/react'
import type { ReactNode } from 'react'
import { AgentAvatar } from '@/components/acp/AgentAvatar'
import { KeyboardShortcut } from '@/components/ui/KeyboardShortcut'
import type { ConnectionSearchResult, ThreadSearchResult } from '@/lib/api/types'
import { useShowModelIcons } from '@/lib/appearance'
import { relativeTime } from '@/lib/format/time'
import { threadSearchTitle } from '@/lib/threadDisplay'
import type { PaletteCommand } from './commandPaletteTypes'

// Rows animate on enter only: a short fade with a hair of upward travel. No
// blur, no layout animation, no stagger, no exit — those are what made the list
// read as jittery. Removing a row just unmounts it; the panel resizes to fit.
const ITEM_TRANSITION: Transition = { duration: 0.16, ease: [0.22, 0.61, 0.36, 1] }
const ITEM_INITIAL = { opacity: 0, y: 5 }
const ITEM_ANIMATE = { opacity: 1, y: 0 }
const SNIPPET_START = '\u001f'
const SNIPPET_END = '\u001e'

type SnippetSegment = {
  text: string
  highlighted: boolean
}

function snippetSegments(text: string): SnippetSegment[] {
  const segments: SnippetSegment[] = []
  let cursor = 0
  while (cursor < text.length) {
    const start = text.indexOf(SNIPPET_START, cursor)
    if (start === -1) {
      segments.push({ text: text.slice(cursor), highlighted: false })
      break
    }
    if (start > cursor) {
      segments.push({ text: text.slice(cursor, start), highlighted: false })
    }
    const end = text.indexOf(SNIPPET_END, start + SNIPPET_START.length)
    if (end === -1) {
      segments.push({ text: text.slice(start), highlighted: false })
      break
    }
    segments.push({
      text: text.slice(start + SNIPPET_START.length, end),
      highlighted: true,
    })
    cursor = end + SNIPPET_END.length
  }
  return segments.filter((segment) => segment.text)
}

function HighlightedSnippet({ text }: { text: string }) {
  if (!text) return null
  return (
    <>
      {snippetSegments(text).map((segment, index) =>
        segment.highlighted ? (
          <mark key={`${segment.text}-${index}`} className="bg-transparent font-medium text-ink">
            {segment.text}
          </mark>
        ) : (
          <span key={`${segment.text}-${index}`}>{segment.text}</span>
        ),
      )}
    </>
  )
}

// Shared shell for every palette row: the enter animation, press feedback,
// active styling, and keyboard-nav hooks all live here so the two row kinds
// only differ in their content.
type PaletteRowProps = {
  active: boolean
  index: number
  reduceMotion: boolean
  onActive: () => void
  onSelect: () => void
  children: ReactNode
}

function PaletteRow({
  active,
  index,
  reduceMotion,
  onActive,
  onSelect,
  children,
}: PaletteRowProps) {
  return (
    <motion.button
      type="button"
      data-command-index={index}
      initial={reduceMotion ? { opacity: 0 } : ITEM_INITIAL}
      animate={ITEM_ANIMATE}
      transition={reduceMotion ? { duration: 0.08 } : ITEM_TRANSITION}
      whileTap={reduceMotion ? undefined : { scale: 0.985 }}
      onClick={onSelect}
      // Select on real pointer movement, not `mouseenter`: as results reflow
      // under a stationary cursor, Chromium re-fires enter on whatever row lands
      // under the pointer, which would yank the highlight off the top result.
      // `mousemove` only fires when the mouse actually moves.
      onMouseMove={onActive}
      // Pointer movement activates the row, so the active highlight is the only
      // highlight. It has no color transition — fading it would make arrow-nav
      // read as laggy.
      className={`flex min-h-8 w-full items-center gap-3 rounded-[10px] px-3 py-1.5 text-left text-[14px] text-ink ${
        active ? 'bg-list-hover' : ''
      }`}
    >
      {children}
    </motion.button>
  )
}

export function CommandRow({
  item,
  active,
  ...row
}: {
  item: PaletteCommand
} & Omit<PaletteRowProps, 'children'>) {
  const Icon = item.icon
  return (
    <PaletteRow {...row} active={active}>
      {Icon ? <Icon size={16} strokeWidth={1.75} className="shrink-0 text-ink-2" /> : null}
      <span className="min-w-0 flex-1 truncate">{item.title}</span>
      {item.shortcut ? (
        <KeyboardShortcut value={item.shortcut} className="border-transparent bg-surface-2" />
      ) : null}
    </PaletteRow>
  )
}

export function ThreadRow({
  result,
  ...row
}: {
  result: ThreadSearchResult
} & Omit<PaletteRowProps, 'children'>) {
  // A title hit's snippet is just the title again; only a message hit adds
  // an excerpt worth a second line.
  const excerpt = result.message_seq ? result.snippet : ''
  const showModelIcons = useShowModelIcons()
  return (
    <PaletteRow {...row}>
      {showModelIcons ? <AgentAvatar agent={result.thread_agent} size={16} className="opacity-60" /> : null}
      <span className="min-w-0 flex-1">
        <span className="block truncate">{threadSearchTitle(result)}</span>
        {excerpt ? (
          <span className="block truncate text-[13px] text-ink-3">
            <HighlightedSnippet text={excerpt} />
          </span>
        ) : null}
      </span>
      <span className="shrink-0 text-[13px] tabular-nums text-ink-3">
        {relativeTime(result.last_attention_at || result.updated_at)}
      </span>
    </PaletteRow>
  )
}

export function ConnectionRow({
  result,
  ...row
}: {
  result: ConnectionSearchResult
} & Omit<PaletteRowProps, 'children'>) {
  return (
    <PaletteRow {...row}>
      <span className="min-w-0 flex-1 truncate">{result.title}</span>
      {result.text ? <span className="max-w-[45%] shrink-0 truncate text-[13px] text-ink-3">{result.text}</span> : null}
    </PaletteRow>
  )
}
