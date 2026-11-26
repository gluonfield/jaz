import { ChevronDown } from 'lucide-react'
import { motion, useReducedMotion } from 'motion/react'
import { useId, useState, type ReactNode } from 'react'
import { Button } from './Button'

const PEEK = 64

// Lays out the first `limit` items and keeps the rest behind a toggle. Collapsed,
// the top of the overflow peeks out of a bottom fade with the toggle lying over
// it, so it reads as part of the list rather than another line of it. The fade
// covers only the toggle's padding band, which is empty once open, so height is
// the only animated value. Collapsed items stay inert: neither focusable nor
// announced.
export function PeekList({ items, limit, className }: { items: ReactNode[]; limit: number; className: string }) {
  const [open, setOpen] = useState(false)
  const id = useId()
  const reduceMotion = useReducedMotion()
  const hidden = items.length - limit
  return (
    <div>
      <div className={className}>{items.slice(0, limit)}</div>
      {hidden > 0 ? (
        <div className="relative">
          <motion.div
            id={id}
            initial={false}
            animate={{ height: open ? 'auto' : PEEK }}
            transition={reduceMotion ? { duration: 0 } : { duration: 0.32, ease: [0.22, 1, 0.36, 1] }}
            inert={!open}
            className="overflow-hidden pb-12 [mask-image:linear-gradient(to_top,transparent_1.5rem,#000_3rem)]"
          >
            <div className={className}>{items.slice(limit)}</div>
          </motion.div>
          <div className="absolute inset-x-0 bottom-0 flex justify-center">
            <Button
              size="sm"
              aria-expanded={open}
              aria-controls={id}
              onClick={() => setOpen(!open)}
              className="border border-border bg-bg shadow-sm"
            >
              {open ? 'Show less' : `Show ${hidden} more`}
              <ChevronDown
                size={14}
                aria-hidden
                className={`transition-transform duration-200 motion-reduce:transition-none ${open ? 'rotate-180' : ''}`}
              />
            </Button>
          </div>
        </div>
      ) : null}
    </div>
  )
}
