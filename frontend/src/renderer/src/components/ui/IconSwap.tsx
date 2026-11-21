import { AnimatePresence, motion, useReducedMotion } from 'motion/react'
import type { ReactNode } from 'react'

// Swaps one icon for another when `id` changes. Render inside a grid parent,
// where the outgoing and incoming icons share a cell.
export function IconSwap({ id, children }: { id: string; children: ReactNode }) {
  const reduceMotion = useReducedMotion()
  return (
    <AnimatePresence initial={false}>
      <motion.span
        key={id}
        className="[grid-area:1/1]"
        initial={reduceMotion ? { opacity: 0 } : { opacity: 0, scale: 0.25, filter: 'blur(4px)' }}
        animate={reduceMotion ? { opacity: 1 } : { opacity: 1, scale: 1, filter: 'blur(0px)' }}
        exit={reduceMotion ? { opacity: 0 } : { opacity: 0, scale: 0.25, filter: 'blur(4px)' }}
        transition={reduceMotion ? { duration: 0.12 } : { type: 'spring', duration: 0.3, bounce: 0 }}
      >
        {children}
      </motion.span>
    </AnimatePresence>
  )
}
