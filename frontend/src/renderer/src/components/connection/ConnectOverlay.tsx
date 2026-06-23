import { AnimatePresence, motion } from 'motion/react'
import { useEffect } from 'react'
import { createPortal } from 'react-dom'
import { LaunchScreen } from '@/components/launch/LaunchScreen'

// Presents the first-run connect screen on demand, over the live app, so
// switching backends after onboarding reuses the exact flow the user
// onboarded with instead of a parallel switcher UI. The connection store does
// the work; this just frames the screen and traps focus while it's open.
export function ConnectOverlay({ open, onClose }: { open: boolean; onClose: () => void }) {
  useEffect(() => {
    if (!open) return
    const root = document.getElementById('root')
    root?.setAttribute('inert', '')
    const onKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        event.stopPropagation()
        onClose()
      }
    }
    document.addEventListener('keydown', onKey, true)
    return () => {
      root?.removeAttribute('inert')
      document.removeEventListener('keydown', onKey, true)
    }
  }, [open, onClose])

  return createPortal(
    <AnimatePresence>
      {open ? (
        <motion.div
          className="fixed inset-0 z-modal"
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          exit={{ opacity: 0 }}
          transition={{ duration: 0.16, ease: 'easeOut' }}
          role="dialog"
          aria-modal="true"
          aria-label="Connect to a machine"
        >
          <LaunchScreen manual onClose={onClose} />
        </motion.div>
      ) : null}
    </AnimatePresence>,
    document.body,
  )
}
