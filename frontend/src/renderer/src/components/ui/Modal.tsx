import { X } from 'lucide-react'
import { AnimatePresence, motion, useReducedMotion } from 'motion/react'
import { useEffect, useRef } from 'react'
import type { ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { IconButton } from '@/components/ui/IconButton'

const SIZES = {
  sm: 'max-w-md',
  md: 'max-w-lg',
  lg: 'max-w-2xl',
} as const

export function Modal({
  open,
  onClose,
  title,
  description,
  icon,
  footer,
  children,
  size = 'md',
}: {
  open: boolean
  onClose: () => void
  title: string
  description?: ReactNode
  icon?: ReactNode
  footer?: ReactNode
  children: ReactNode
  size?: keyof typeof SIZES
}) {
  const reduce = useReducedMotion()
  const panelRef = useRef<HTMLDivElement>(null)

  // Auto-focus the first field only as the modal opens. Keeping this off the
  // onClose dependency stops re-renders (e.g. typing) from yanking focus back.
  useEffect(() => {
    if (!open) return
    const previouslyFocused = document.activeElement as HTMLElement | null
    const panel = panelRef.current
    const firstField = panel?.querySelector<HTMLElement>('input, textarea, select')
    ;(firstField ?? panel)?.focus()
    return () => previouslyFocused?.focus?.()
  }, [open])

  useEffect(() => {
    if (!open) return
    const onKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        // An open transient surface inside the panel (popover menu, mention
        // suggestions) owns Escape — it dismisses itself via its own listener,
        // and the modal only closes on the next press.
        if (panelRef.current?.querySelector('[data-escape-surface]')) return
        event.stopPropagation()
        onClose()
        return
      }
      if (event.key !== 'Tab') return
      const panel = panelRef.current
      if (!panel) return
      const focusable = panel.querySelectorAll<HTMLElement>(
        'a[href],button:not([disabled]),input:not([disabled]),textarea:not([disabled]),select:not([disabled]),[tabindex]:not([tabindex="-1"])',
      )
      if (focusable.length === 0) return
      const first = focusable[0]
      const last = focusable[focusable.length - 1]
      const active = document.activeElement
      if (event.shiftKey && active === first) {
        event.preventDefault()
        last.focus()
      } else if (!event.shiftKey && active === last) {
        event.preventDefault()
        first.focus()
      }
    }
    document.addEventListener('keydown', onKey, true)
    return () => document.removeEventListener('keydown', onKey, true)
  }, [open, onClose])

  return createPortal(
    <AnimatePresence>
      {open ? (
        <motion.div
          className="fixed inset-0 z-[60] overflow-y-auto bg-black/40 backdrop-blur-[2px]"
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          exit={{ opacity: 0 }}
          transition={{ duration: 0.15, ease: 'easeOut' }}
          onClick={onClose}
        >
          <div className="flex min-h-full items-center justify-center p-4 sm:p-6">
            <motion.div
              ref={panelRef}
              role="dialog"
              aria-modal="true"
              aria-label={title}
              tabIndex={-1}
              onClick={(event) => event.stopPropagation()}
              initial={reduce ? { opacity: 0 } : { opacity: 0, y: 8, scale: 0.98 }}
              animate={{ opacity: 1, y: 0, scale: 1 }}
              exit={reduce ? { opacity: 0 } : { opacity: 0, y: 6, scale: 0.98 }}
              transition={{ type: 'spring', stiffness: 460, damping: 34 }}
              className={`flex max-h-[calc(100dvh-2rem)] w-full ${SIZES[size]} flex-col overflow-hidden rounded-card border border-border bg-bg shadow-lg sm:max-h-[calc(100dvh-3rem)]`}
            >
              <header className="flex items-start gap-3 border-b border-border px-5 py-4">
                {icon ? (
                  <div className="grid size-9 shrink-0 place-items-center rounded-control bg-surface-2 text-ink-2">
                    {icon}
                  </div>
                ) : null}
                <div className="min-w-0 flex-1">
                  <h2 className="text-sm font-semibold text-ink">{title}</h2>
                  {description ? (
                    <p className="mt-0.5 text-[13px] text-ink-2">{description}</p>
                  ) : null}
                </div>
                <IconButton
                  variant="ghost"
                  size="sm"
                  onClick={onClose}
                  aria-label="Close"
                  className="-mr-1.5 -mt-1"
                >
                  <X size={16} />
                </IconButton>
              </header>

              <div className="min-h-0 flex-1 overflow-y-auto px-5 py-5">{children}</div>

              {footer ? (
                <footer className="flex items-center justify-between gap-3 border-t border-border px-5 py-3">
                  {footer}
                </footer>
              ) : null}
            </motion.div>
          </div>
        </motion.div>
      ) : null}
    </AnimatePresence>,
    document.body,
  )
}
