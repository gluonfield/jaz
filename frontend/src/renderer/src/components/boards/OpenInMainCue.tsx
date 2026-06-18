import { ExternalLink } from 'lucide-react'
import { AnimatePresence, motion, useReducedMotion } from 'motion/react'
import { createPortal } from 'react-dom'

// A small inverse pill that flashes just above a click point and fades out.
// Opening a loop from a popped-out board window hands off to the main app,
// which can sit behind the board — so the cue is anchored to the cursor (where
// the eye already is) rather than a bottom-right toast that's easy to miss.
// Portaled to the body so the board's overflow clip can't shave it.
export function OpenInMainCue({ point }: { point: { x: number; y: number } | null }) {
  const reduce = useReducedMotion()
  // Centering (-50%) and lift (-100%) are constant across every variant, so
  // motion holds the translate steady and only opacity + scale animate. The
  // -12px on `top` adds the gap above the click that -100% leaves at the edge.
  const anchor = { x: '-50%', y: '-100%' }
  return createPortal(
    <AnimatePresence>
      {point ? (
        <motion.div
          key="open-cue"
          className="pointer-events-none fixed z-tooltip flex items-center gap-1.5 whitespace-nowrap rounded-full bg-ink px-2.5 py-1 text-[11px] font-medium text-bg shadow-[var(--shadow-raised)]"
          style={{ left: point.x, top: point.y - 12 }}
          initial={reduce ? { ...anchor, opacity: 0 } : { ...anchor, opacity: 0, scale: 0.96 }}
          animate={{ ...anchor, opacity: 1, scale: 1 }}
          exit={reduce ? { ...anchor, opacity: 0 } : { ...anchor, opacity: 0, scale: 0.98 }}
          transition={reduce ? { duration: 0.1 } : { type: 'spring', stiffness: 420, damping: 30 }}
        >
          <ExternalLink size={11} className="shrink-0 opacity-80" />
          Opening in main Jaz app
        </motion.div>
      ) : null}
    </AnimatePresence>,
    document.body,
  )
}
