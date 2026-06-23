import { MonitorSmartphone, Server } from 'lucide-react'
import { AnimatePresence, motion } from 'motion/react'
import { useState } from 'react'
import { createPortal } from 'react-dom'
import { RAINBOW_BEAM } from '@/components/ui/rainbow'
import { useBackendChange } from '@/lib/connection'
import { type BackendDescription, backendName, describeBackend } from '@/lib/connectionDisplay'

const HOLD_MS = 1000

// A brief "jump to another machine" flourish whenever the active backend
// changes — it masks the content reload underneath (cache clear + route reset)
// and names where you landed. Mounted above the router so it also plays across
// a fresh backend's onboarding. Only a real switch fires it; the first connect
// and reconnecting to the same backend stay quiet.
export function BackendTransition() {
  const [target, setTarget] = useState<BackendDescription | null>(null)

  useBackendChange((url) => {
    setTarget({ ...describeBackend(url), title: backendName(url) })
    const timer = window.setTimeout(() => setTarget(null), HOLD_MS)
    return () => window.clearTimeout(timer)
  })

  return createPortal(
    <AnimatePresence>{target ? <TransitionScene backend={target} /> : null}</AnimatePresence>,
    document.body,
  )
}

function TransitionScene({ backend }: { backend: BackendDescription }) {
  const Icon = backend.local ? MonitorSmartphone : Server
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
          className="pointer-events-none absolute left-1/2 top-1/2 size-32 -translate-x-1/2 -translate-y-1/2 rounded-full ring-1 ring-primary/30"
          initial={{ scale: 0.5, opacity: 0 }}
          animate={{ scale: 3.4, opacity: [0, 0.5, 0] }}
          transition={{ duration: 0.9, delay, ease: 'easeOut' }}
        />
      ))}

      <div className="relative flex flex-col items-center gap-4">
        <motion.div
          className="relative grid size-[84px] place-items-center"
          initial={{ scale: 0.4, opacity: 0, filter: 'blur(6px)' }}
          animate={{ scale: 1, opacity: 1, filter: 'blur(0px)' }}
          exit={{ scale: 1.12, opacity: 0 }}
          transition={{ type: 'spring', stiffness: 300, damping: 18 }}
        >
          {/* the same rainbow comet the composer wears, orbiting the machine */}
          <motion.div
            aria-hidden
            className="pointer-events-none absolute -inset-[3px] rounded-full"
            style={{ background: RAINBOW_BEAM }}
            initial={{ opacity: 0 }}
            animate={{ opacity: 1, '--ring-angle': ['0deg', '360deg'] }}
            transition={{
              opacity: { duration: 0.3, ease: 'easeOut' },
              '--ring-angle': { duration: 1.4, ease: 'linear', repeat: Infinity },
            }}
          />
          <div className="absolute inset-0 rounded-full bg-surface shadow-raised" />
          <Icon size={32} className="relative text-ink" />
        </motion.div>

        <motion.div
          className="flex flex-col items-center gap-1 text-center"
          initial={{ opacity: 0, y: 12 }}
          animate={{ opacity: 1, y: 0 }}
          exit={{ opacity: 0 }}
          transition={{ delay: 0.14, duration: 0.32, ease: 'easeOut' }}
        >
          <p className="text-[10px] font-semibold uppercase tracking-[0.16em] text-ink-3">Switched to</p>
          <p className="text-balance text-[22px] font-semibold tracking-tight text-ink">{backend.title}</p>
          <p className="max-w-[80vw] truncate font-mono text-[11px] text-ink-3">{backend.url}</p>
        </motion.div>
      </div>
    </motion.div>
  )
}
