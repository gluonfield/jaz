import { MessageSquare } from 'lucide-react'
import { motion, type Transition } from 'motion/react'
import { KeyboardShortcut } from '@/components/ui/KeyboardShortcut'
import type { ThreadSearchResult } from '@/lib/api/types'
import { relativeTime } from '@/lib/format/time'
import type { PaletteCommand } from './commandPaletteTypes'
import { threadTitle } from './commandPaletteTypes'

const ITEM_TRANSITION: Transition = { type: 'spring', duration: 0.22, bounce: 0 }
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
            className="rounded-[5px] bg-primary-soft px-0.5 text-primary-strong"
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

export function CommandRow({
  item,
  active,
  index,
  reduceMotion,
  onActive,
  onSelect,
}: {
  item: PaletteCommand
  active: boolean
  index: number
  reduceMotion: boolean
  onActive: () => void
  onSelect: () => void
}) {
  const Icon = item.Icon
  return (
    <motion.button
      type="button"
      data-command-index={index}
      layout
      initial={reduceMotion ? { opacity: 0 } : { opacity: 0, y: 4, filter: 'blur(3px)' }}
      animate={{ opacity: 1, y: 0, filter: 'blur(0px)' }}
      exit={reduceMotion ? { opacity: 0 } : { opacity: 0, y: -2, filter: 'blur(2px)' }}
      transition={reduceMotion ? { duration: 0.08 } : { ...ITEM_TRANSITION, delay: Math.min(index, 6) * 0.012 }}
      whileTap={reduceMotion ? undefined : { scale: 0.96 }}
      onClick={onSelect}
      onMouseEnter={onActive}
      className={`group flex min-h-11 w-full items-center gap-2.5 rounded-[10px] px-2.5 text-left transition-colors duration-150 ${
        active ? 'bg-surface text-ink shadow-[inset_0_0_0_1px_var(--color-border)]' : 'hover:bg-surface/70'
      }`}
    >
      <span className={`grid size-7 shrink-0 place-items-center rounded-[8px] ${active ? 'bg-bg' : 'bg-surface'}`}>
        <Icon size={15} className={active ? 'text-ink' : 'text-ink-3'} />
      </span>
      <span className="min-w-0 flex-1">
        <span className="block truncate text-[13px] font-medium text-ink">{item.title}</span>
        <span className="block truncate text-[12px] text-ink-3">{item.detail}</span>
      </span>
      {item.shortcut ? <KeyboardShortcut value={item.shortcut} className="bg-surface" /> : null}
    </motion.button>
  )
}

export function ThreadRow({
  result,
  active,
  index,
  reduceMotion,
  onActive,
  onSelect,
}: {
  result: ThreadSearchResult
  active: boolean
  index: number
  reduceMotion: boolean
  onActive: () => void
  onSelect: () => void
}) {
  return (
    <motion.button
      type="button"
      data-command-index={index}
      layout
      initial={reduceMotion ? { opacity: 0 } : { opacity: 0, y: 4, filter: 'blur(3px)' }}
      animate={{ opacity: 1, y: 0, filter: 'blur(0px)' }}
      exit={reduceMotion ? { opacity: 0 } : { opacity: 0, y: -2, filter: 'blur(2px)' }}
      transition={reduceMotion ? { duration: 0.08 } : { ...ITEM_TRANSITION, delay: Math.min(index, 6) * 0.012 }}
      whileTap={reduceMotion ? undefined : { scale: 0.96 }}
      onClick={onSelect}
      onMouseEnter={onActive}
      className={`group flex min-h-[58px] w-full items-start gap-2.5 rounded-[10px] px-2.5 py-2 text-left transition-colors duration-150 ${
        active ? 'bg-surface text-ink shadow-[inset_0_0_0_1px_var(--color-border)]' : 'hover:bg-surface/70'
      }`}
    >
      <span className={`mt-0.5 grid size-7 shrink-0 place-items-center rounded-[8px] ${active ? 'bg-bg' : 'bg-surface'}`}>
        <MessageSquare size={15} className="text-ink-3" />
      </span>
      <span className="min-w-0 flex-1">
        <span className="flex min-w-0 items-center">
          <span className="truncate text-[13px] font-medium text-ink">{threadTitle(result)}</span>
        </span>
        <span className="mt-0.5 line-clamp-1 text-[12px] leading-5 text-ink-2">
          <HighlightedSnippet text={result.snippet || result.thread_slug} />
        </span>
      </span>
      <span className="mt-1 shrink-0 text-[11px] tabular-nums text-ink-3">
        {relativeTime(result.last_attention_at || result.updated_at)}
      </span>
    </motion.button>
  )
}
