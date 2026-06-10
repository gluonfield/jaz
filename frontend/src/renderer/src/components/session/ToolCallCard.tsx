import { ChevronRight } from 'lucide-react'
import { AnimatePresence, motion } from 'motion/react'
import { useState } from 'react'

function prettyArgs(raw: string): string {
  try {
    return JSON.stringify(JSON.parse(raw), null, 2)
  } catch {
    return raw
  }
}

// Minimal inline affordance: just a chevron + tool name in the flow of the
// conversation. Expanding reveals a bordered panel with arguments and result.
export function ToolCallCard({
  name,
  args,
  result,
  pending = false,
}: {
  name: string
  args?: string
  result?: string
  pending?: boolean
}) {
  const [open, setOpen] = useState(false)
  const hasDetails = Boolean(args || result)

  return (
    <div className="flex flex-col items-start gap-1.5">
      <button
        type="button"
        disabled={!hasDetails}
        aria-expanded={open}
        onClick={() => setOpen((o) => !o)}
        className="flex cursor-pointer items-center gap-1 font-mono text-[12px] text-ink-3 transition-colors duration-150 enabled:hover:text-ink disabled:cursor-default"
      >
        <motion.span
          animate={{ rotate: open ? 90 : 0 }}
          transition={{ duration: 0.15, ease: 'easeOut' }}
          className={hasDetails ? '' : 'opacity-30'}
        >
          <ChevronRight size={12} />
        </motion.span>
        {name}
        {pending ? (
          <span className="ml-1.5 inline-flex items-center gap-1 text-[11px]">
            <span className="jaz-shimmer size-1.5 rounded-full" />
            running
          </span>
        ) : null}
      </button>

      <AnimatePresence initial={false}>
        {open && hasDetails ? (
          <motion.div
            initial={{ height: 0, opacity: 0 }}
            animate={{ height: 'auto', opacity: 1 }}
            exit={{ height: 0, opacity: 0 }}
            transition={{ duration: 0.18, ease: 'easeOut' }}
            className="w-full overflow-hidden"
          >
            <div className="overflow-hidden rounded-card border border-border">
              {args ? (
                <pre className="max-h-44 overflow-auto px-3 py-2 font-mono text-[12px] leading-relaxed whitespace-pre-wrap text-ink-2 select-text">
                  {prettyArgs(args)}
                </pre>
              ) : null}
              {args && result ? <div className="border-t border-border" /> : null}
              {result ? (
                <pre className="max-h-44 overflow-auto px-3 py-2 font-mono text-[12px] leading-relaxed whitespace-pre-wrap text-ink-2 select-text">
                  {result}
                </pre>
              ) : null}
            </div>
          </motion.div>
        ) : null}
      </AnimatePresence>
    </div>
  )
}
