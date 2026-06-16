import { useNavigate } from '@tanstack/react-router'
import { Search, X } from 'lucide-react'
import { AnimatePresence, motion, useReducedMotion, type Transition } from 'motion/react'
import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type KeyboardEvent as ReactKeyboardEvent,
} from 'react'
import { createPortal } from 'react-dom'
import { RAINBOW_BEAM } from '@/components/ui/rainbow'
import type { PaletteItem } from './commandPaletteTypes'
import { CommandRow, ThreadRow } from './CommandPaletteRows'
import { useCommandPaletteItems } from './useCommandPaletteItems'

// Panel enters with a quick, calm spring; no bounce so it never feels rubbery.
const PANEL_TRANSITION: Transition = { type: 'spring', duration: 0.26, bounce: 0 }
// Section labels fade in with their section.
const LABEL_TRANSITION: Transition = { duration: 0.14, ease: 'easeOut' }

export function CommandPalette({
  open,
  onOpenChange,
  onOpenSettings,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  onOpenSettings: () => void
}) {
  const navigate = useNavigate()
  const reduceMotion = useReducedMotion()
  const inputRef = useRef<HTMLInputElement>(null)
  const listRef = useRef<HTMLDivElement>(null)
  const [query, setQuery] = useState('')
  const [activeIndex, setActiveIndex] = useState(0)
  const { debouncedQuery, items, commandItems, threadItems, searchEnabled, threadSearch } =
    useCommandPaletteItems({
      open,
      query,
      onOpenChange,
      onOpenSettings,
    })

  const close = useCallback(() => onOpenChange(false), [onOpenChange])

  const selectItem = useCallback(
    (item: PaletteItem | undefined) => {
      if (!item) return
      if (item.kind === 'command') {
        item.run()
        return
      }
      close()
      navigate({
        to: '/sessions/$sessionId',
        params: { sessionId: item.result.thread_id },
        search: item.result.message_seq ? { message: item.result.message_seq } : {},
      })
    },
    [close, navigate],
  )

  // While the palette is open it owns focus. The rest of the app is marked
  // `inert` so nothing behind the backdrop (e.g. the home-screen composer) can
  // be focused or clicked — the palette is the only thing the pointer and
  // keyboard can reach. On close we un-inert *before* restoring focus, so the
  // previously focused element is interactive again when we hand focus back.
  useEffect(() => {
    if (!open) return
    const root = document.getElementById('root')
    const previous = document.activeElement as HTMLElement | null
    root?.setAttribute('inert', '')
    requestAnimationFrame(() => inputRef.current?.focus())
    return () => {
      root?.removeAttribute('inert')
      previous?.focus?.()
    }
  }, [open])

  // Never reopen mid-search.
  useEffect(() => {
    if (!open) setQuery('')
  }, [open])

  useEffect(() => {
    if (!open) return
    setActiveIndex(0)
  }, [debouncedQuery, items.length, open])

  useEffect(() => {
    if (!open) return
    const active = listRef.current?.querySelector<HTMLElement>(
      `[data-command-index="${activeIndex}"]`,
    )
    active?.scrollIntoView({ block: 'nearest' })
  }, [activeIndex, open])

  const onPaletteKeyDown = (event: ReactKeyboardEvent<HTMLElement>) => {
    if (event.key === 'Escape') {
      event.preventDefault()
      close()
      return
    }
    if (event.key === 'ArrowDown') {
      event.preventDefault()
      setActiveIndex((index) => (items.length ? (index + 1) % items.length : 0))
      return
    }
    if (event.key === 'ArrowUp') {
      event.preventDefault()
      setActiveIndex((index) => (items.length ? (index - 1 + items.length) % items.length : 0))
      return
    }
    if (event.key === 'Enter') {
      if (event.target !== inputRef.current) return
      event.preventDefault()
      selectItem(items[activeIndex])
    }
  }

  const showSkeleton = threadSearch.isFetching && searchEnabled && threadItems.length === 0
  const showNoMatches = !threadSearch.isFetching && searchEnabled && items.length === 0
  const showEmpty = !searchEnabled && items.length === 0

  return createPortal(
    <AnimatePresence initial={false}>
      {open ? (
        <motion.div
          className="fixed inset-0 z-command bg-black/25 px-3 pt-[9dvh] backdrop-blur-[1.5px]"
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          exit={{ opacity: 0 }}
          transition={{ duration: reduceMotion ? 0.08 : 0.14, ease: [0.2, 0, 0, 1] }}
          onMouseDown={close}
        >
          <motion.div
            role="dialog"
            aria-modal="true"
            aria-label="Command palette"
            onKeyDown={onPaletteKeyDown}
            onMouseDown={(event) => event.stopPropagation()}
            initial={reduceMotion ? { opacity: 0 } : { opacity: 0, y: -8, scale: 0.982 }}
            animate={{ opacity: 1, y: 0, scale: 1 }}
            exit={reduceMotion ? { opacity: 0 } : { opacity: 0, y: -4, scale: 0.99 }}
            transition={PANEL_TRANSITION}
            className="relative mx-auto w-full max-w-[620px]"
          >
            {/* Rainbow comet border — the same "alive right now" ring the
                composer wears while focused, orbiting the palette the whole
                time it's open. It rides this wrapper (not the clipped surface),
                so the comet shows on the perimeter while the opaque surface
                covers its center. */}
            <motion.div
              aria-hidden
              className="pointer-events-none absolute -inset-[2px]"
              initial={{ opacity: 0 }}
              animate={{
                opacity: 1,
                ...(reduceMotion ? {} : { '--ring-angle': ['0deg', '360deg'] }),
              }}
              transition={{
                opacity: { duration: 0.25, ease: 'easeOut' },
                '--ring-angle': { duration: 2.6, ease: 'linear', repeat: Infinity },
              }}
            >
              {/* glow trailing the comet, bleeding softly outside the panel */}
              <div
                className="absolute -inset-[4px] rounded-[18px] opacity-50 blur-[10px]"
                style={{ background: RAINBOW_BEAM }}
              />
              {/* the comet itself; the panel's opaque surface covers the center */}
              <div className="absolute inset-0 rounded-[14px]" style={{ background: RAINBOW_BEAM }} />
            </motion.div>

            <div className="relative flex max-h-[min(590px,76dvh)] flex-col overflow-hidden rounded-[12px] bg-bg shadow-[0_18px_48px_rgba(0,0,0,0.22),0_2px_8px_rgba(0,0,0,0.08)]">
              <div className="flex items-center gap-2 px-3 py-2.5">
              <Search size={17} className="shrink-0 text-ink-3" />
              <input
                ref={inputRef}
                value={query}
                onChange={(event) => setQuery(event.currentTarget.value)}
                placeholder="Search threads, actions, settings"
                aria-label="Search threads or run a command"
                className="h-9 min-w-0 flex-1 bg-transparent text-[14px] text-ink outline-none placeholder:text-ink-3"
              />
              {query ? (
                <button
                  type="button"
                  aria-label="Clear search"
                  title="Clear search"
                  onClick={() => {
                    setQuery('')
                    inputRef.current?.focus()
                  }}
                  className="relative grid size-8 shrink-0 place-items-center rounded-[6px] text-ink-3 transition-colors duration-150 before:absolute before:-inset-1 before:content-[''] hover:bg-surface hover:text-ink"
                >
                  <X size={15} />
                </button>
              ) : null}
            </div>

            <div ref={listRef} className="min-h-0 flex-1 overflow-y-auto overscroll-contain px-1.5 py-1.5">
              {commandItems.length ? (
                <div className="px-2.5 pb-1 pt-1.5 text-[10px] font-semibold uppercase tracking-[0.08em] text-ink-3">
                  Actions
                </div>
              ) : null}
              {commandItems.map((item, index) => (
                <CommandRow
                  key={item.id}
                  item={item}
                  active={index === activeIndex}
                  index={index}
                  reduceMotion={Boolean(reduceMotion)}
                  onActive={() => setActiveIndex(index)}
                  onSelect={() => selectItem(item)}
                />
              ))}

              {threadItems.length ? (
                <motion.div
                  initial={reduceMotion ? { opacity: 0 } : { opacity: 0, y: 2 }}
                  animate={{ opacity: 1, y: 0 }}
                  transition={LABEL_TRANSITION}
                  className="px-2.5 pb-1 pt-2.5 text-[10px] font-semibold uppercase tracking-[0.08em] text-ink-3"
                >
                  Threads
                </motion.div>
              ) : null}
              {threadItems.map((item, index) => {
                const itemIndex = commandItems.length + index
                return (
                  <ThreadRow
                    key={item.id}
                    result={item.result}
                    active={itemIndex === activeIndex}
                    index={itemIndex}
                    reduceMotion={Boolean(reduceMotion)}
                    onActive={() => setActiveIndex(itemIndex)}
                    onSelect={() => selectItem(item)}
                  />
                )
              })}

              {showSkeleton ? (
                <div className="flex flex-col gap-1 px-0.5 pb-1 pt-1.5">
                  {[0, 1, 2].map((row) => (
                    <motion.div
                      key={row}
                      initial={false}
                      animate={reduceMotion ? { opacity: 0.5 } : { opacity: [0.4, 0.65, 0.4] }}
                      transition={
                        reduceMotion ? { duration: 0 } : { repeat: Infinity, duration: 1.2, delay: row * 0.08 }
                      }
                      className="h-[52px] rounded-[6px] bg-surface"
                    />
                  ))}
                </div>
              ) : null}
              {showNoMatches ? (
                <div className="grid min-h-28 place-items-center px-6 text-center">
                  <p className="text-[13px] text-ink-3">No thread matches "{debouncedQuery}".</p>
                </div>
              ) : null}
              {showEmpty ? (
                <div className="grid min-h-24 place-items-center px-6 text-center">
                  <p className="text-[13px] text-ink-3">No results.</p>
                </div>
              ) : null}
            </div>
            </div>
          </motion.div>
        </motion.div>
      ) : null}
    </AnimatePresence>,
    document.body,
  )
}
