import { AnimatePresence, motion, useReducedMotion } from 'motion/react'
import { useEffect, useMemo, useState, type RefObject } from 'react'
import { createPortal } from 'react-dom'
import type { ChatMessage, SessionEvent } from '@/lib/api/types'
import { layoutRect, layoutViewport } from '@/lib/dom/zoom'
import { buildOutline, type OutlineEntry } from './outline'

// Tick length reads as answer volume, so the rail doubles as a density map of
// the thread. Rest is uniform and quiet; pointing at the rail resolves the
// lengths, pointing at a tick opens its preview.
const TICK_REST = 8
const TICK_MIN = 12
const TICK_MAX = 24
const TICK_HOVER = 28
const TICK_PITCH = 10
const RAIL_MAX_HEIGHT = 340

const CARD_WIDTH = 300
const CARD_GAP = 14
const CARD_MAX_HEIGHT = 152
const VIEWPORT_MARGIN = 12

// The turn in view is the last one whose top has crossed the reading line.
const READING_LINE = 0.3

type Anchor = { seq: number; left: number; top: number }

function useActiveEntry(
  entries: OutlineEntry[],
  scrollRef: RefObject<HTMLDivElement | null>,
): number | undefined {
  const [seq, setSeq] = useState<number>()
  useEffect(() => {
    const root = scrollRef.current
    if (!root) return
    const tracked = new Set(entries.map((entry) => entry.seq))
    let frame = 0
    const measure = () => {
      frame = 0
      const line = root.getBoundingClientRect().top + root.clientHeight * READING_LINE
      let current: number | undefined
      for (const node of root.querySelectorAll<HTMLElement>('[data-message-seq]')) {
        if (node.getBoundingClientRect().top > line) break
        const value = Number(node.dataset.messageSeq)
        if (tracked.has(value)) current = value
      }
      setSeq(current)
    }
    const onScroll = () => {
      if (!frame) frame = requestAnimationFrame(measure)
    }
    measure()
    root.addEventListener('scroll', onScroll, { passive: true })
    return () => {
      root.removeEventListener('scroll', onScroll)
      if (frame) cancelAnimationFrame(frame)
    }
  }, [entries, scrollRef])
  return seq
}

export function ThreadOutline({
  messages,
  events,
  scrollRef,
  onSelect,
}: {
  messages: ChatMessage[]
  events: SessionEvent[]
  scrollRef: RefObject<HTMLDivElement | null>
  onSelect: (seq: number) => void
}) {
  const entries = useMemo(() => buildOutline(messages, events), [messages, events])
  const [anchor, setAnchor] = useState<Anchor | null>(null)
  const [railActive, setRailActive] = useState(false)
  const activeSeq = useActiveEntry(entries, scrollRef)

  // One turn navigates to itself; the rail earns its space from two.
  if (entries.length < 2) return null

  const open = (element: HTMLElement, seq: number) => {
    const rect = layoutRect(element)
    const half = CARD_MAX_HEIGHT / 2
    const limit = layoutViewport().height - VIEWPORT_MARGIN - half
    setRailActive(true)
    setAnchor({
      seq,
      left: rect.left + TICK_HOVER + CARD_GAP,
      top: Math.min(Math.max(rect.top + rect.height / 2, VIEWPORT_MARGIN + half), limit),
    })
  }
  const close = () => {
    setRailActive(false)
    setAnchor(null)
  }

  return (
    <>
      {/* Parks in the gutter left of the thread column, sliding inward only as
          far as the column's own padding when the pane gets narrow. */}
      <div
        className="pointer-events-none absolute inset-y-0 z-dropdown flex items-center max-sm:hidden"
        style={{ left: 'clamp(6px, calc((100% - var(--thread-max)) / 2 + 6px), 20px)' }}
      >
        <nav
          aria-label="Thread outline"
          onMouseEnter={() => setRailActive(true)}
          onMouseLeave={close}
          style={{ height: Math.min(entries.length * TICK_PITCH, RAIL_MAX_HEIGHT) }}
          className="pointer-events-auto flex flex-col justify-between"
        >
          {entries.map((entry) => {
            const hovered = anchor?.seq === entry.seq
            const active = activeSeq === entry.seq
            const length = hovered
              ? TICK_HOVER
              : railActive
                ? TICK_MIN + entry.weight * (TICK_MAX - TICK_MIN)
                : TICK_REST
            return (
              <button
                key={entry.seq}
                type="button"
                aria-current={active ? 'true' : undefined}
                onMouseEnter={(event) => open(event.currentTarget, entry.seq)}
                onFocus={(event) => open(event.currentTarget, entry.seq)}
                onBlur={close}
                onClick={() => onSelect(entry.seq)}
                className={`flex h-2.5 min-h-0 shrink cursor-pointer items-center pr-3 transition-colors duration-200 motion-reduce:transition-none ${
                  hovered
                    ? 'text-primary'
                    : active
                      ? 'text-ink-2'
                      : railActive
                        ? 'text-ink-3/70'
                        : 'text-ink-3/35'
                }`}
              >
                <span
                  className="h-0.5 w-7 origin-left rounded-full bg-current transition-transform duration-200 motion-reduce:transition-none"
                  style={{ transform: `scaleX(${length / TICK_HOVER})` }}
                />
                <span className="sr-only">Jump to: {entry.title}</span>
              </button>
            )
          })}
        </nav>
      </div>
      <OutlineCard entry={entries.find((entry) => entry.seq === anchor?.seq)} anchor={anchor} />
    </>
  )
}

function OutlineCard({ entry, anchor }: { entry?: OutlineEntry; anchor: Anchor | null }) {
  const reduce = useReducedMotion()
  return createPortal(
    <AnimatePresence>
      {entry && anchor ? (
        <motion.div
          key="thread-outline-card"
          initial={{ opacity: 0, x: reduce ? 0 : -6, y: '-50%' }}
          animate={{ opacity: 1, x: 0, y: '-50%' }}
          exit={{ opacity: 0, x: reduce ? 0 : -6, y: '-50%' }}
          transition={{ duration: 0.14, ease: 'easeOut' }}
          style={{ left: anchor.left, top: anchor.top, width: CARD_WIDTH }}
          className="pointer-events-none fixed z-tooltip rounded-card bg-surface p-3.5 shadow-raised ring-1 ring-border/70"
        >
          <p className="truncate text-[13px] font-medium text-ink">{entry.title}</p>
          {entry.preview.map((paragraph, index) => (
            <p
              key={index}
              className={`mt-1.5 text-[12.5px] leading-relaxed ${
                index ? 'line-clamp-2 text-ink-3' : 'line-clamp-1 text-ink-2'
              }`}
            >
              {paragraph}
            </p>
          ))}
        </motion.div>
      ) : null}
    </AnimatePresence>,
    document.body,
  )
}
