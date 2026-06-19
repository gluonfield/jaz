import { AnimatePresence, motion, useReducedMotion } from 'motion/react'
import { useEffect } from 'react'
import { createPortal } from 'react-dom'
import { LoopExamples } from './LoopExamples'
import type { LoopTemplate } from './loopTemplates'

// A small floating panel of example loops, layered over the loop modal without
// a dimming backdrop so the modal stays visible behind it. Portaled to escape
// the modal's overflow/transform; dismissed on outside-click or Escape.
export function LoopExamplesPicker({
  open,
  onClose,
  onPick,
}: {
  open: boolean
  onClose: () => void
  onPick: (template: LoopTemplate) => void
}) {
  const reduce = useReducedMotion()

  useEffect(() => {
    if (!open) return
    // Listen on window in the capture phase so Escape closes the picker before
    // the loop modal's own document-level handler can close the modal.
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== 'Escape') return
      e.stopPropagation()
      onClose()
    }
    window.addEventListener('keydown', onKey, true)
    return () => window.removeEventListener('keydown', onKey, true)
  }, [open, onClose])

  return createPortal(
    <AnimatePresence>
      {open ? (
        <div
          className="fixed inset-0 z-modal flex items-center justify-center p-4"
          onClick={onClose}
        >
          <motion.div
            role="dialog"
            aria-label="Examples"
            onClick={(e) => e.stopPropagation()}
            initial={reduce ? { opacity: 0 } : { opacity: 0, y: 8, scale: 0.98 }}
            animate={{ opacity: 1, y: 0, scale: 1 }}
            exit={reduce ? { opacity: 0 } : { opacity: 0, y: 6, scale: 0.98 }}
            transition={{ type: 'spring', stiffness: 460, damping: 34 }}
            className="flex max-h-[70dvh] w-full max-w-xs flex-col overflow-hidden rounded-card bg-bg p-1.5 shadow-raised ring-1 ring-border"
          >
            <div className="px-2 pb-1 pt-1.5 text-[11px] font-medium uppercase tracking-wide text-ink-3">
              Examples
            </div>
            <div className="min-h-0 flex-1 overflow-y-auto">
              <LoopExamples onPick={onPick} />
            </div>
          </motion.div>
        </div>
      ) : null}
    </AnimatePresence>,
    document.body,
  )
}
