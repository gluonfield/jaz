import { Bot, Clock, MessageSquare, User } from 'lucide-react'
import { motion, type Transition } from 'motion/react'
import { KeyboardShortcut } from '@/components/ui/KeyboardShortcut'
import type { ThreadSearchResult } from '@/lib/api/types'
import { relativeTime } from '@/lib/format/time'
import type { PaletteCommand, PaletteItem, RoleMode } from './commandPaletteTypes'
import { threadTitle } from './commandPaletteTypes'

const ITEM_TRANSITION: Transition = { duration: 0.16, ease: [0.2, 0, 0, 1] }
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

function roleLabel(role?: string): string {
  switch (role) {
    case 'user':
      return 'User'
    case 'assistant':
      return 'Assistant'
    default:
      return 'Thread'
  }
}

function ResultIcon({ result }: { result: ThreadSearchResult }) {
  if (result.role === 'user') return <User size={16} className="text-primary" />
  if (result.role === 'assistant') return <Bot size={16} className="text-primary" />
  return <MessageSquare size={16} className="text-primary" />
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

export function RoleToggle({
  mode,
  value,
  children,
  onChange,
}: {
  mode: RoleMode
  value: RoleMode
  children: string
  onChange: (mode: RoleMode) => void
}) {
  const active = mode === value
  return (
    <button
      type="button"
      aria-pressed={active}
      onClick={() => onChange(value)}
      className={`h-7 rounded-full px-3 text-[12px] font-medium transition-colors duration-150 ${
        active
          ? 'bg-primary text-on-primary shadow-sm'
          : 'text-ink-2 hover:bg-surface-2 hover:text-ink'
      }`}
    >
      {children}
    </button>
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
      initial={reduceMotion ? { opacity: 0 } : { opacity: 0, y: 5, filter: 'blur(3px)' }}
      animate={{ opacity: 1, y: 0, filter: 'blur(0px)' }}
      exit={reduceMotion ? { opacity: 0 } : { opacity: 0, y: -3, filter: 'blur(3px)' }}
      transition={ITEM_TRANSITION}
      onClick={onSelect}
      onMouseEnter={onActive}
      className={`group flex min-h-14 w-full items-center gap-3 rounded-control px-3 text-left transition-colors duration-150 ${
        active ? 'bg-primary-soft text-ink' : 'hover:bg-surface'
      }`}
    >
      <span className={`grid size-8 shrink-0 place-items-center rounded-full ${active ? 'bg-bg' : 'bg-surface'}`}>
        <Icon size={16} className="text-primary" />
      </span>
      <span className="min-w-0 flex-1">
        <span className="block truncate text-[13px] font-medium text-ink">{item.title}</span>
        <span className="block truncate text-[12px] text-ink-3">{item.detail}</span>
      </span>
      {item.shortcut ? <KeyboardShortcut value={item.shortcut} /> : null}
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
      initial={reduceMotion ? { opacity: 0 } : { opacity: 0, y: 5, filter: 'blur(3px)' }}
      animate={{ opacity: 1, y: 0, filter: 'blur(0px)' }}
      exit={reduceMotion ? { opacity: 0 } : { opacity: 0, y: -3, filter: 'blur(3px)' }}
      transition={ITEM_TRANSITION}
      onClick={onSelect}
      onMouseEnter={onActive}
      className={`group flex min-h-[68px] w-full items-start gap-3 rounded-control px-3 py-2.5 text-left transition-colors duration-150 ${
        active ? 'bg-primary-soft text-ink' : 'hover:bg-surface'
      }`}
    >
      <span className={`mt-0.5 grid size-8 shrink-0 place-items-center rounded-full ${active ? 'bg-bg' : 'bg-surface'}`}>
        <ResultIcon result={result} />
      </span>
      <span className="min-w-0 flex-1">
        <span className="flex min-w-0 items-center gap-2">
          <span className="truncate text-[13px] font-medium text-ink">{threadTitle(result)}</span>
          <span className="shrink-0 rounded-full bg-bg px-1.5 py-[2px] text-[10px] font-medium text-ink-3">
            {roleLabel(result.role)}
          </span>
        </span>
        <span className="mt-1 line-clamp-2 text-[12px] leading-5 text-ink-2">
          <HighlightedSnippet text={result.snippet || result.thread_slug} />
        </span>
      </span>
      <span className="mt-1 flex shrink-0 items-center gap-1 text-[11px] tabular-nums text-ink-3">
        <Clock size={12} />
        {relativeTime(result.last_attention_at || result.updated_at)}
      </span>
    </motion.button>
  )
}

export function PaletteFooter({ activeItem }: { activeItem?: PaletteItem }) {
  return (
    <div className="flex items-center justify-between border-t border-border px-3 py-2 text-[11px] text-ink-3">
      <div className="flex items-center gap-1.5">
        <span className="rounded-full bg-bg px-2 py-1 font-mono">Up/Down</span>
        <span>Move</span>
        <span className="ml-2 rounded-full bg-bg px-2 py-1 font-mono">Enter</span>
        <span>Open</span>
      </div>
      <span className="max-w-[45%] truncate">
        {activeItem?.kind === 'thread' ? activeItem.result.thread_slug : activeItem?.detail}
      </span>
    </div>
  )
}
