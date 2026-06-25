import { ChevronDown } from 'lucide-react'
import { AnimatePresence, motion } from 'motion/react'
import { type ReactNode, useMemo, useState } from 'react'
import { createPortal } from 'react-dom'
import { useWindowEvent } from '@/lib/hooks/useWindowEvent'
import { useTitlebarSlot } from '@/lib/titlebar'

// Phone-only: the new-thread controls (agent, model, project, worktree) don't
// fit beside the composer, so they collapse behind a single header trigger that
// drops a panel down from the title bar. The panel and its dismiss backdrop
// portal to the body so the controls' own popovers can layer above it.
export function NewThreadOptions({
  title,
  subtitle,
  children,
}: {
  title: string
  subtitle?: string
  children: ReactNode
}) {
  const [open, setOpen] = useState(false)

  // Dismiss when the route changes underneath an open panel (e.g. a send that
  // navigates) is handled by unmount; here we only close on backdrop/Escape.
  useWindowEvent(
    'keydown',
    (e) => {
      if (e.key === 'Escape') setOpen(false)
    },
    open,
  )

  // Centered on the viewport: fixed so the title-bar slot's own positioning
  // context can't pull it to the slot's left edge.
  const trigger = useMemo(
    () => (
      <button
        type="button"
        aria-haspopup="dialog"
        aria-expanded={open}
        onClick={() => setOpen((v) => !v)}
        className="fixed top-3.5 left-1/2 flex -translate-x-1/2 flex-col items-center rounded-full px-2.5 py-1 leading-tight [-webkit-app-region:no-drag] hover:bg-surface-2"
      >
        {/* The label is the centered element; the chevron hangs off its right edge so it never pulls the text off-center. */}
        <span className="relative flex items-center text-[13px] font-medium text-ink">
          <span className="max-w-[60vw] truncate">{title}</span>
          <ChevronDown
            size={13}
            className={`absolute top-1/2 left-full ml-0.5 -translate-y-1/2 text-ink-3 transition-transform duration-150 ${open ? 'rotate-180' : ''}`}
          />
        </span>
        {subtitle ? <span className="max-w-[60vw] truncate text-[11px] text-ink-3">{subtitle}</span> : null}
      </button>
    ),
    [open, subtitle, title],
  )
  useTitlebarSlot(trigger)

  return (
    <>
      {createPortal(
        <AnimatePresence>
          {open ? (
            <>
              <motion.div
                key="backdrop"
                className="fixed inset-0 z-modal"
                initial={{ opacity: 0 }}
                animate={{ opacity: 1 }}
                exit={{ opacity: 0 }}
                transition={{ duration: 0.15 }}
                onClick={() => setOpen(false)}
              />
              <motion.div
                key="panel"
                role="dialog"
                aria-label="Thread settings"
                className="fixed inset-x-2 top-[60px] z-modal rounded-[14px] bg-surface p-3 shadow-xl ring-1 ring-border"
                initial={{ opacity: 0, y: -8 }}
                animate={{ opacity: 1, y: 0 }}
                exit={{ opacity: 0, y: -8 }}
                transition={{ duration: 0.15, ease: 'easeOut' }}
              >
                <div className="flex flex-wrap items-center gap-2">{children}</div>
              </motion.div>
            </>
          ) : null}
        </AnimatePresence>,
        document.body,
      )}
    </>
  )
}
