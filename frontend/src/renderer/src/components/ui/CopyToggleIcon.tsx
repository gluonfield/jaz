import { Check, Copy } from 'lucide-react'
import { AnimatePresence, motion, useReducedMotion } from 'motion/react'

// Animated copy → check swap shared by copy buttons. Render inside a button with
// the `group` class so the idle copy icon can pick up the button's hover state.
export function CopyToggleIcon({ copied }: { copied: boolean }) {
  const reduceMotion = useReducedMotion()
  return (
    <span className="grid size-3.5 shrink-0 place-items-center">
      <AnimatePresence initial={false} mode="popLayout">
        <motion.span
          key={copied ? 'copied' : 'copy'}
          initial={reduceMotion ? { opacity: 0 } : { opacity: 0, scale: 0.25, filter: 'blur(4px)' }}
          animate={reduceMotion ? { opacity: 1 } : { opacity: 1, scale: 1, filter: 'blur(0px)' }}
          exit={reduceMotion ? { opacity: 0 } : { opacity: 0, scale: 0.25, filter: 'blur(4px)' }}
          transition={reduceMotion ? { duration: 0.12 } : { type: 'spring', duration: 0.3, bounce: 0 }}
          className="grid size-3.5 place-items-center"
        >
          {copied ? (
            <Check size={14} className="text-primary" aria-hidden />
          ) : (
            <Copy size={14} className="text-ink-3 transition-colors group-hover:text-ink" aria-hidden />
          )}
        </motion.span>
      </AnimatePresence>
    </span>
  )
}
