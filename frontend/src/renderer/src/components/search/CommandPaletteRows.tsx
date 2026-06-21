import { Archive } from 'lucide-react'
import { motion, type Transition } from 'motion/react'
import type { ReactNode } from 'react'
import { AgentAvatar } from '@/components/acp/AgentAvatar'
import { KeyboardShortcut } from '@/components/ui/KeyboardShortcut'
import type { ThreadSearchResult } from '@/lib/api/types'
import { relativeTime } from '@/lib/format/time'
import type { PaletteCommand } from './commandPaletteTypes'
import { threadTitle } from './commandPaletteTypes'

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
          <mark
            key={`${segment.text}-${index}`}
            className="rounded-[3px] bg-primary-soft px-0.5 text-primary-strong"
          >
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
// active/hover styling, and keyboard-nav hooks all live here so the two row
// kinds only differ in their content and layout (height/alignment) classes.
type PaletteRowProps = {
  active: boolean
  index: number
  reduceMotion: boolean
  onActive: () => void
  onSelect: () => void
  className: string
  children: ReactNode
}

function PaletteRow({
  active,
  index,
  reduceMotion,
  onActive,
  onSelect,
  className,
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
      onMouseEnter={onActive}
      // Selection must snap on keypress, so the highlight has no color
      // transition — fading it would make arrow-nav read as laggy. Hover keeps
      // a hair of fade since the pointer moves continuously.
      className={`group flex w-full gap-2 rounded-[6px] px-2.5 text-left ${className} ${
        active
          ? 'bg-surface text-ink'
          : 'text-ink transition-colors duration-100 hover:bg-surface/70'
      }`}
    >
      {children}
    </motion.button>
  )
}

export function CommandRow({
  item,
  ...row
}: {
  item: PaletteCommand
} & Omit<PaletteRowProps, 'className' | 'children'>) {
  return (
    <PaletteRow {...row} className="min-h-[52px] items-center py-2">
      <span className="min-w-0 flex-1">
        <span className="block truncate text-[13px] font-medium text-ink">{item.title}</span>
        <span className="block truncate text-[12px] text-ink-3">{item.detail}</span>
      </span>
      {item.shortcut ? <KeyboardShortcut value={item.shortcut} className="bg-surface-2" /> : null}
    </PaletteRow>
  )
}

export function ThreadRow({
  result,
  ...row
}: {
  result: ThreadSearchResult
} & Omit<PaletteRowProps, 'className' | 'children'>) {
  return (
    <PaletteRow {...row} className="min-h-[52px] items-start py-2">
      <span className="min-w-0 flex-1">
        <span className="flex min-w-0 items-center gap-1.5">
          <AgentAvatar agent={result.thread_agent} size={16} />
          {result.archived ? (
            <span
              title="Archived"
              className="grid size-[18px] shrink-0 place-items-center rounded-[4px] bg-surface text-ink-3"
            >
              <Archive size={11} aria-label="Archived" />
            </span>
          ) : null}
          <span
            className={`truncate text-[13px] font-medium ${result.archived ? 'text-ink-2' : 'text-ink'}`}
          >
            {threadTitle(result)}
          </span>
        </span>
        <span className="mt-0.5 line-clamp-1 text-[12px] leading-5 text-ink-2">
          <HighlightedSnippet text={result.snippet || result.thread_slug} />
        </span>
      </span>
      <span className="mt-1 shrink-0 text-[11px] tabular-nums text-ink-3">
        {relativeTime(result.last_attention_at || result.updated_at)}
      </span>
    </PaletteRow>
  )
}
