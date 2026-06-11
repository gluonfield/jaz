import { type ReactNode } from 'react'
import { motion } from 'motion/react'

export function BottomDock({ before, children }: { before?: ReactNode; children: ReactNode }) {
  return (
    <div className="pointer-events-none absolute inset-x-0 bottom-0 bg-gradient-to-b from-transparent to-bg to-45% px-10 pt-6 pb-5">
      <motion.div
        className="pointer-events-auto mx-auto max-w-[640px]"
        initial={{ y: 12, opacity: 0 }}
        animate={{ y: 0, opacity: 1 }}
        transition={{ type: 'spring', stiffness: 380, damping: 32 }}
      >
        {before}
        {children}
      </motion.div>
    </div>
  )
}
