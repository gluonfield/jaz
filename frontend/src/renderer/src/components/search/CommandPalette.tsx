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
import type { PaletteItem } from './commandPaletteTypes'
import {
  CommandRow,
  ThreadRow,
} from './CommandPaletteRows'
import { useCommandPaletteItems } from './useCommandPaletteItems'

const PANEL_TRANSITION: Transition = { type: 'spring', duration: 0.28, bounce: 0 }

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
  const {
    debouncedQuery,
    items,
    searchEnabled,
    threadSearch,
  } = useCommandPaletteItems({
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

  useEffect(() => {
    if (!open) return
    const previous = document.activeElement as HTMLElement | null
    requestAnimationFrame(() => inputRef.current?.focus())
    return () => previous?.focus?.()
  }, [open])

  useEffect(() => {
    if (!open) return
    setActiveIndex(0)
  }, [debouncedQuery, items.length, open])

  useEffect(() => {
    if (!open) return
    const active = listRef.current?.querySelector<HTMLElement>(`[data-command-index="${activeIndex}"]`)
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
  const indexedItems = items.map((item, index) => ({ item, index }))
  const commandItems = indexedItems.filter(({ item }) => item.kind === 'command')
  const threadItems = indexedItems.filter(({ item }) => item.kind === 'thread')

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
            exit={reduceMotion ? { opacity: 0 } : { opacity: 0, y: -4, scale: 0.988 }}
            transition={PANEL_TRANSITION}
            className="mx-auto flex max-h-[min(590px,76dvh)] w-full max-w-[640px] flex-col overflow-hidden rounded-[14px] bg-bg shadow-[0_24px_70px_rgba(0,0,0,0.24),0_2px_10px_rgba(0,0,0,0.08)] ring-1 ring-border"
          >
            <div className="flex items-center gap-2 px-3 py-2.5 shadow-[inset_0_-1px_0_var(--color-border)]">
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
                  onClick={() => setQuery('')}
                  className="relative grid size-8 shrink-0 place-items-center rounded-[9px] text-ink-3 transition-colors duration-150 before:absolute before:-inset-1 before:content-[''] hover:bg-surface hover:text-ink"
                >
                  <X size={15} />
                </button>
              ) : null}
            </div>

            <div ref={listRef} className="min-h-0 flex-1 overflow-y-auto px-1.5 py-1.5">
              {commandItems.length ? (
                <div className="px-2 pb-1 pt-2 text-[10px] font-semibold uppercase tracking-[0.08em] text-ink-3">
                  Actions
                </div>
              ) : null}
              <AnimatePresence initial={false}>
                {commandItems.map(({ item, index }) => (
                  item.kind === 'command' ? (
                    <CommandRow
                      key={item.id}
                      item={item}
                      active={index === activeIndex}
                      index={index}
                      reduceMotion={Boolean(reduceMotion)}
                      onActive={() => setActiveIndex(index)}
                      onSelect={() => selectItem(item)}
                    />
                  ) : null
                ))}
              </AnimatePresence>
              {threadItems.length ? (
                <div className="px-2 pb-1 pt-2 text-[10px] font-semibold uppercase tracking-[0.08em] text-ink-3">
                  Threads
                </div>
              ) : null}
              <AnimatePresence initial={false}>
                {threadItems.map(({ item, index }) => (
                  item.kind === 'thread' ? (
                    <ThreadRow
                      key={item.id}
                      result={item.result}
                      active={index === activeIndex}
                      index={index}
                      reduceMotion={Boolean(reduceMotion)}
                      onActive={() => setActiveIndex(index)}
                      onSelect={() => selectItem(item)}
                    />
                  ) : null
                ))}
              </AnimatePresence>
              {threadSearch.isFetching && searchEnabled ? (
                <div className="flex flex-col gap-1 px-0.5 py-1">
                  {[0, 1, 2].map((row) => (
                    <motion.div
                      key={row}
                      initial={false}
                      animate={{ opacity: [0.38, 0.66, 0.38] }}
                      transition={{ repeat: Infinity, duration: 1.1, delay: row * 0.07 }}
                      className="h-12 rounded-[10px] bg-surface"
                    />
                  ))}
                </div>
              ) : null}
              {!threadSearch.isFetching && searchEnabled && items.length === 0 ? (
                <div className="grid min-h-28 place-items-center px-6 text-center">
                  <p className="text-[13px] text-ink-3">No thread matches "{debouncedQuery}".</p>
                </div>
              ) : null}
              {!searchEnabled && items.length === 0 ? (
                <div className="grid min-h-24 place-items-center px-6 text-center">
                  <p className="text-[13px] text-ink-3">No results.</p>
                </div>
              ) : null}
            </div>
          </motion.div>
        </motion.div>
      ) : null}
    </AnimatePresence>,
    document.body,
  )
}
