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
import { KeyboardShortcut } from '@/components/ui/KeyboardShortcut'
import type { PaletteItem, RoleMode } from './commandPaletteTypes'
import {
  CommandRow,
  PaletteFooter,
  RoleToggle,
  ThreadRow,
} from './CommandPaletteRows'
import { useCommandPaletteItems } from './useCommandPaletteItems'

const PANEL_TRANSITION: Transition = { type: 'spring', stiffness: 520, damping: 40 }

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
  const [roleMode, setRoleMode] = useState<RoleMode>('all')
  const [activeIndex, setActiveIndex] = useState(0)
  const {
    debouncedQuery,
    items,
    searchEnabled,
    threadSearch,
  } = useCommandPaletteItems({
    open,
    query,
    roleMode,
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
  }, [debouncedQuery, items.length, open, roleMode])

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

  return createPortal(
    <AnimatePresence initial={false}>
      {open ? (
        <motion.div
          className="fixed inset-0 z-command bg-black/35 px-3 pt-[12dvh] backdrop-blur-[2px]"
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          exit={{ opacity: 0 }}
          transition={{ duration: reduceMotion ? 0.08 : 0.16, ease: [0.2, 0, 0, 1] }}
          onMouseDown={close}
        >
          <motion.div
            role="dialog"
            aria-modal="true"
            aria-label="Command palette"
            onKeyDown={onPaletteKeyDown}
            onMouseDown={(event) => event.stopPropagation()}
            initial={reduceMotion ? { opacity: 0 } : { opacity: 0, y: -10, scale: 0.985 }}
            animate={{ opacity: 1, y: 0, scale: 1 }}
            exit={reduceMotion ? { opacity: 0 } : { opacity: 0, y: -6, scale: 0.985 }}
            transition={PANEL_TRANSITION}
            className="mx-auto flex max-h-[min(680px,76dvh)] w-full max-w-[720px] flex-col overflow-hidden rounded-[18px] bg-bg shadow-[0_24px_80px_rgba(0,0,0,0.26)] ring-1 ring-border"
          >
            <div className="flex items-center gap-3 border-b border-border px-4 py-3">
              <Search size={18} className="shrink-0 text-primary" />
              <input
                ref={inputRef}
                value={query}
                onChange={(event) => setQuery(event.currentTarget.value)}
                placeholder="Search threads or run a command"
                aria-label="Search threads or run a command"
                className="h-9 min-w-0 flex-1 bg-transparent text-[15px] text-ink outline-none placeholder:text-ink-3"
              />
              {query ? (
                <button
                  type="button"
                  aria-label="Clear search"
                  title="Clear search"
                  onClick={() => setQuery('')}
                  className="grid size-8 shrink-0 place-items-center rounded-full text-ink-3 transition-colors duration-150 hover:bg-surface-2 hover:text-ink"
                >
                  <X size={15} />
                </button>
              ) : (
                <KeyboardShortcut value="K" />
              )}
            </div>

            <div className="flex items-center border-b border-border px-3 py-2">
              <div className="flex items-center rounded-full bg-surface p-0.5">
                <RoleToggle mode={roleMode} value="all" onChange={setRoleMode}>
                  All
                </RoleToggle>
                <RoleToggle mode={roleMode} value="user" onChange={setRoleMode}>
                  User
                </RoleToggle>
                <RoleToggle mode={roleMode} value="assistant" onChange={setRoleMode}>
                  Assistant
                </RoleToggle>
              </div>
            </div>

            <div ref={listRef} className="min-h-0 flex-1 overflow-y-auto p-2">
              <AnimatePresence initial={false} mode="popLayout">
                {items.map((item, index) =>
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
                  ) : (
                    <ThreadRow
                      key={item.id}
                      result={item.result}
                      active={index === activeIndex}
                      index={index}
                      reduceMotion={Boolean(reduceMotion)}
                      onActive={() => setActiveIndex(index)}
                      onSelect={() => selectItem(item)}
                    />
                  ),
                )}
              </AnimatePresence>
              {threadSearch.isFetching && searchEnabled ? (
                <div className="flex flex-col gap-1 px-1 py-1">
                  {[0, 1, 2].map((row) => (
                    <motion.div
                      key={row}
                      initial={false}
                      animate={{ opacity: [0.45, 0.7, 0.45] }}
                      transition={{ repeat: Infinity, duration: 1.2, delay: row * 0.08 }}
                      className="h-14 rounded-control bg-surface"
                    />
                  ))}
                </div>
              ) : null}
              {!threadSearch.isFetching && searchEnabled && items.length === 0 ? (
                <div className="grid min-h-36 place-items-center px-6 text-center">
                  <p className="text-[13px] text-ink-3">No thread matches "{debouncedQuery}".</p>
                </div>
              ) : null}
              {!searchEnabled && items.length === 0 ? (
                <div className="grid min-h-28 place-items-center px-6 text-center">
                  <p className="text-[13px] text-ink-3">Type at least two characters.</p>
                </div>
              ) : null}
            </div>

            <PaletteFooter activeItem={items[activeIndex]} />
          </motion.div>
        </motion.div>
      ) : null}
    </AnimatePresence>,
    document.body,
  )
}
