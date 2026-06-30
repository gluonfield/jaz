import { AnimatePresence, motion, type HTMLMotionProps, type Transition } from 'motion/react'
import { forwardRef, type ReactNode } from 'react'

const itemTransition = { type: 'spring', duration: 0.3, bounce: 0 } satisfies Transition

export function AnimatedList({ children }: { children: ReactNode }) {
  return (
    <AnimatePresence initial={false} mode="popLayout">
      {children}
    </AnimatePresence>
  )
}

export const AnimatedListItem = forwardRef<
  HTMLDivElement,
  Omit<HTMLMotionProps<'div'>, 'layout' | 'exit' | 'transition'>
>(function AnimatedListItem({ children, ...props }, ref) {
  return (
    <motion.div
      ref={ref}
      {...props}
      layout="position"
      exit={{ opacity: 0, x: -8 }}
      transition={itemTransition}
    >
      {children}
    </motion.div>
  )
})
