import { useNavigate } from '@tanstack/react-router'
import { X } from 'lucide-react'
import { AnimatePresence, motion, useReducedMotion, type Transition } from 'motion/react'
import {
  Fragment,
  useCallback,
  useEffect,
  useRef,
  useState,
  type KeyboardEvent as ReactKeyboardEvent,
} from 'react'
import { createPortal } from 'react-dom'
import { AppIcon } from '@/components/apps/AppIcon'
import type { SettingsSection } from '@/components/settings/sections'
import type { PaletteItem } from './commandPaletteTypes'
import { CommandRow, ConnectionRow, ThreadRow } from './CommandPaletteRows'
import { useCommandPaletteItems } from './useCommandPaletteItems'

// Panel enters with a quick, calm spring; no bounce so it never feels rubbery.
const PANEL_TRANSITION: Transition = { type: 'spring', duration: 0.26, bounce: 0 }
// Section labels fade in with their section.
const LABEL_TRANSITION: Transition = { duration: 0.14, ease: 'easeOut' }

export function CommandPalette({
  open,
  onOpenChange,
  onOpenSettings,
  onOpenConnect,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  onOpenSettings: (section?: SettingsSection) => void
  onOpenConnect: () => void
}) {
  const navigate = useNavigate()
  const reduceMotion = useReducedMotion()
  const inputRef = useRef<HTMLInputElement>(null)
  const listRef = useRef<HTMLDivElement>(null)
  const [query, setQuery] = useState('')
  const [activeIndex, setActiveIndex] = useState(0)
  const { debouncedQuery, items, commandItems, sections, searchEnabled, searching } =
    useCommandPaletteItems({
      open,
      query,
      onOpenChange,
      onOpenSettings,
      onOpenConnect,
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
      if (item.kind === 'connection') {
        window.open(item.result.url, '_blank', 'noopener,noreferrer')
        return
      }
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

  // Snap focus to the top result the instant the query changes (not the
  // debounced value, which would leave the highlight on a stale row for ~140ms)
  // and again whenever async results arrive and resize the list.
  useEffect(() => {
    if (!open) return
    setActiveIndex(0)
  }, [query, items.length, open])

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

  const showSkeleton = searching && searchEnabled && items.length === commandItems.length
  const showNoMatches = !searching && searchEnabled && items.length === 0
  const showEmpty = !searchEnabled && items.length === 0
  const rowProps = (item: PaletteItem) => {
    const index = items.indexOf(item)
    return {
      active: index === activeIndex,
      index,
      reduceMotion: Boolean(reduceMotion),
      onActive: () => setActiveIndex(index),
      onSelect: () => selectItem(item),
    }
  }

  return createPortal(
    <AnimatePresence initial={false}>
      {open ? (
        <motion.div
          className="fixed inset-0 z-command flex items-start justify-center overflow-hidden bg-black/25 px-3 py-[10dvh]"
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
            className="flex max-h-[min(590px,76dvh)] w-full max-w-[620px] flex-col overflow-hidden rounded-[18px] bg-bg shadow-[0_24px_60px_-12px_rgba(0,0,0,0.32),0_2px_8px_rgba(0,0,0,0.06)] ring-1 ring-border/70 dark:bg-surface"
          >
            <div className="flex items-center gap-2 px-5 pb-1 pt-3">
              <input
                ref={inputRef}
                value={query}
                onChange={(event) => setQuery(event.currentTarget.value)}
                placeholder="Search or run a command"
                aria-label="Search or run a command"
                className="h-10 min-w-0 flex-1 bg-transparent text-[16px] text-ink outline-none placeholder:text-ink-3"
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
                  className="relative -mr-2 grid size-8 shrink-0 place-items-center rounded-full text-ink-3 transition-colors duration-150 before:absolute before:-inset-1 before:content-[''] hover:bg-list-hover hover:text-ink"
                >
                  <X size={15} />
                </button>
              ) : null}
            </div>

            <div ref={listRef} className="min-h-0 flex-1 overflow-y-auto overscroll-contain px-2 pb-2">
              {commandItems.length ? (
                <div className="px-3 pb-1 pt-2 text-[13px] text-ink-3">Actions</div>
              ) : null}
              {commandItems.map((item) => (
                <CommandRow key={item.id} item={item} {...rowProps(item)} />
              ))}

              {sections.map((section) => (
                <Fragment key={section.id}>
                  <motion.div
                    initial={reduceMotion ? { opacity: 0 } : { opacity: 0, y: 2 }}
                    animate={{ opacity: 1, y: 0 }}
                    transition={LABEL_TRANSITION}
                    className="flex items-center gap-2 px-3 pb-1 pt-3 text-[13px] text-ink-3"
                  >
                    {section.app ? <AppIcon app={section.app} size={14} /> : null}
                    {section.label}
                  </motion.div>
                  {section.items.map((item) =>
                    item.kind === 'thread' ? (
                      <ThreadRow key={item.id} result={item.result} {...rowProps(item)} />
                    ) : (
                      <ConnectionRow key={item.id} result={item.result} {...rowProps(item)} />
                    ),
                  )}
                </Fragment>
              ))}

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
                      className="h-8 rounded-[10px] bg-list-hover"
                    />
                  ))}
                </div>
              ) : null}
              {showNoMatches ? (
                <div className="grid min-h-28 place-items-center px-6 text-center">
                  <p className="text-[13px] text-ink-3">No matches for "{debouncedQuery}".</p>
                </div>
              ) : null}
              {showEmpty ? (
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
