import { AnimatePresence, motion } from 'motion/react'
import { useState } from 'react'
import { createPortal } from 'react-dom'
import { useBackendChange } from '@/lib/connection'
import { backendName } from '@/lib/connectionDisplay'

const HOLD_MS = 1000

// A brief "jump to another machine" flourish whenever the active backend
// changes — it masks the content reload underneath (cache clear + route reset)
// and names where you landed. Mounted above the router so it also plays across
// a fresh backend's onboarding. Only a real switch fires it; the first connect
// and reconnecting to the same backend stay quiet.
export function BackendTransition() {
  const [name, setName] = useState<string | null>(null)

  useBackendChange((url) => {
    setName(backendName(url))
    const timer = window.setTimeout(() => setName(null), HOLD_MS)
    return () => window.clearTimeout(timer)
  })

  return createPortal(
    <AnimatePresence>{name ? <TransitionScene name={name} /> : null}</AnimatePresence>,
    document.body,
  )
}

function TransitionScene({ name }: { name: string }) {
  return (
    <motion.div
      className="fixed inset-0 z-[110] grid place-items-center overflow-hidden bg-bg"
      initial={{ opacity: 0 }}
      animate={{ opacity: 1 }}
      exit={{ opacity: 0 }}
      transition={{ duration: 0.22, ease: 'easeOut' }}
    >
      {/* a wash of brand color blooming out from the center */}
      <motion.div
        aria-hidden
        className="pointer-events-none absolute left-1/2 top-1/2 size-[140vmax] -translate-x-1/2 -translate-y-1/2 rounded-full"
        style={{ background: 'radial-gradient(circle, var(--color-primary-soft) 0%, transparent 55%)' }}
        initial={{ scale: 0.2, opacity: 0 }}
        animate={{ scale: [0.2, 1, 1.2], opacity: [0, 0.7, 0] }}
        transition={{ duration: 0.9, ease: 'easeOut' }}
      />
      {/* teleport pulse rings rippling outward */}
      {[0, 0.12, 0.24].map((delay) => (
        <motion.span
          key={delay}
          aria-hidden
          className="pointer-events-none absolute left-1/2 top-1/2 size-40 -translate-x-1/2 -translate-y-1/2 rounded-full ring-1 ring-primary/30"
          initial={{ scale: 0.5, opacity: 0 }}
          animate={{ scale: 3, opacity: [0, 0.5, 0] }}
          transition={{ duration: 0.9, delay, ease: 'easeOut' }}
        />
      ))}

      <div className="relative flex flex-col items-center gap-1.5 text-center">
        <motion.p
          className="text-[10px] font-semibold uppercase tracking-[0.18em] text-ink-3"
          initial={{ opacity: 0, y: 8 }}
          animate={{ opacity: 1, y: 0 }}
          exit={{ opacity: 0 }}
          transition={{ duration: 0.3, ease: 'easeOut' }}
        >
          Switched to
        </motion.p>
        <motion.p
          className="max-w-[80vw] text-balance px-4 text-[30px] font-semibold tracking-tight text-ink"
          initial={{ opacity: 0, scale: 0.72, filter: 'blur(8px)' }}
          animate={{ opacity: 1, scale: 1, filter: 'blur(0px)' }}
          exit={{ opacity: 0, scale: 1.06 }}
          transition={{ type: 'spring', stiffness: 280, damping: 18 }}
        >
          {name}
        </motion.p>
      </div>
    </motion.div>
  )
}
