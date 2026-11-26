import { ChevronDown } from 'lucide-react'
import { motion, useReducedMotion } from 'motion/react'
import { useId, useState, type ReactNode } from 'react'
import { Button } from './Button'

const PEEK = 64

// Overflow below an always-visible preview. Collapsed, its top edge peeks out
// under a bottom fade with the toggle lying over it, so the hidden content
// reads as part of the container rather than another line of it. Height and
// fade animate together; the toggle rides the bottom edge in both states.
// Collapsed content stays inert, so it is neither focusable nor announced.
export function PeekCollapse({ hiddenCount, children }: { hiddenCount: number; children: ReactNode }) {
  const [open, setOpen] = useState(false)
  const id = useId()
  const reduceMotion = useReducedMotion()
  return (
    <div className="relative">
      <motion.div
        id={id}
        initial={false}
        animate={open ? { height: 'auto', '--peek-fade': '0px' } : { height: PEEK, '--peek-fade': `${PEEK}px` }}
        transition={reduceMotion ? { duration: 0 } : { duration: 0.32, ease: [0.22, 1, 0.36, 1] }}
        inert={!open}
        className="overflow-hidden [mask-image:linear-gradient(to_top,transparent_calc(var(--peek-fade)/3),#000_var(--peek-fade))]"
      >
        <div className="pb-10">{children}</div>
      </motion.div>
      <div className="absolute inset-x-0 bottom-0 flex justify-center">
        <Button
          size="sm"
          aria-expanded={open}
          aria-controls={id}
          onClick={() => setOpen(!open)}
          className="border border-border bg-bg px-3 shadow-sm"
        >
          {open ? 'Show less' : `Show ${hiddenCount} more`}
          <ChevronDown
            size={14}
            aria-hidden
            className={`transition-transform duration-200 motion-reduce:transition-none ${open ? 'rotate-180' : ''}`}
          />
        </Button>
      </div>
    </div>
  )
}
