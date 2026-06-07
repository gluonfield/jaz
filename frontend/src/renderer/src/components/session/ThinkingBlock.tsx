import { ChevronRight } from 'lucide-react'
import { AnimatePresence, motion } from 'motion/react'
import { useState } from 'react'

export function ThinkingBlock({ text, pending = false }: { text: string; pending?: boolean }) {
  const [open, setOpen] = useState(false)
  const trimmed = text.trim()
  if (!trimmed) return null

  return (
    <div className="flex flex-col items-start gap-1.5">
      <button
        type="button"
        aria-expanded={open}
        onClick={() => setOpen((value) => !value)}
        className="flex cursor-pointer items-center gap-1 font-mono text-[12px] text-ink-3 transition-colors duration-150 hover:text-ink"
      >
        <motion.span
          animate={{ rotate: open ? 90 : 0 }}
          transition={{ duration: 0.15, ease: 'easeOut' }}
        >
          <ChevronRight size={12} />
        </motion.span>
        Thinking
        {pending ? (
          <span className="ml-1.5 inline-flex items-center gap-1 text-[11px]">
            <span className="size-1.5 animate-pulse rounded-full bg-running" />
            live
          </span>
        ) : null}
      </button>

      <AnimatePresence initial={false}>
        {open ? (
          <motion.div
            initial={{ height: 0, opacity: 0 }}
            animate={{ height: 'auto', opacity: 1 }}
            exit={{ height: 0, opacity: 0 }}
            transition={{ duration: 0.18, ease: 'easeOut' }}
            className="w-full overflow-hidden"
          >
            <pre className="max-h-56 overflow-auto rounded-card border border-border bg-surface px-3 py-2 font-mono text-[12px] leading-relaxed whitespace-pre-wrap text-ink-2 select-text">
              {trimmed}
            </pre>
          </motion.div>
        ) : null}
      </AnimatePresence>
    </div>
  )
}
