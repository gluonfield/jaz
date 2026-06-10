import { Check } from 'lucide-react'
import { AnimatePresence, motion, useReducedMotion } from 'motion/react'
import { type ReactNode, useEffect, useRef } from 'react'

// A floating menu anchored to its trigger, dismissed on outside-click/Escape.
// Trigger and menu share one wrapper so clicking the trigger doesn't self-close.
export function Popover({
  open,
  onClose,
  trigger,
  children,
  placement = 'above',
}: {
  open: boolean
  onClose: () => void
  trigger: ReactNode
  children: ReactNode
  placement?: 'above' | 'below'
}) {
  const ref = useRef<HTMLDivElement>(null)
  const reducedMotion = useReducedMotion()

  useEffect(() => {
    if (!open) return
    const onDown = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) onClose()
    }
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    document.addEventListener('mousedown', onDown)
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('mousedown', onDown)
      document.removeEventListener('keydown', onKey)
    }
  }, [open, onClose])

  const slide = reducedMotion ? 0 : placement === 'above' ? 6 : -6
  return (
    <div ref={ref} className="relative">
      {trigger}
      <AnimatePresence>
        {open ? (
          <motion.div
            initial={{ opacity: 0, y: slide }}
            animate={{ opacity: 1, y: 0 }}
            exit={{ opacity: 0, y: slide }}
            transition={{ duration: 0.15, ease: 'easeOut' }}
            // no-drag keeps the panel clickable when it overlaps the titlebar
            // drag region (harmless everywhere else).
            className={`absolute left-0 z-20 min-w-[176px] rounded-[14px] bg-surface p-1.5 shadow-xl ring-1 ring-border [-webkit-app-region:no-drag] ${
              placement === 'above' ? 'bottom-full mb-1.5' : 'top-full mt-1.5'
            }`}
          >
            {children}
          </motion.div>
        ) : null}
      </AnimatePresence>
    </div>
  )
}

export function MenuRow({
  selected,
  onClick,
  children,
}: {
  selected?: boolean
  onClick: () => void
  children: ReactNode
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={`flex h-7 w-full items-center gap-2 rounded-full px-2.5 text-left text-[13px] transition-colors duration-150 hover:bg-surface-2 ${
        selected ? 'text-ink' : 'text-ink-2'
      }`}
    >
      <span className="min-w-0 flex-1 truncate">{children}</span>
      {selected ? <Check size={13} className="shrink-0 text-primary" /> : null}
    </button>
  )
}
