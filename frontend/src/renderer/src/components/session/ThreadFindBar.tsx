import { ChevronDown, ChevronUp, Search, X } from 'lucide-react'
import { AnimatePresence, motion } from 'motion/react'
import type { ThreadFindController } from '@/components/session/useThreadFind'

export function ThreadFindBar({
  find,
}: {
  find: ThreadFindController
}) {
  return (
    <AnimatePresence initial={false}>
      {find.open ? (
        <motion.div
          data-thread-find-ignore
          initial={{ opacity: 0, y: -6, scale: 0.98 }}
          animate={{ opacity: 1, y: 0, scale: 1 }}
          exit={{ opacity: 0, y: -6, scale: 0.98 }}
          transition={{ type: 'spring', duration: 0.3, bounce: 0 }}
          className="absolute top-3 right-4 z-dropdown flex h-10 max-w-[calc(100%-2rem)] items-center gap-1 rounded-full bg-surface px-2 shadow-[0_8px_30px_rgba(0,0,0,0.16)]"
        >
          <Search size={15} className="ml-1 shrink-0 text-ink-3" aria-hidden />
          <input
            ref={find.inputRef}
            value={find.query}
            onChange={(event) => find.setQuery(event.currentTarget.value)}
            placeholder="Find in thread"
            aria-label="Find in thread"
            className="h-8 w-[min(220px,42vw)] bg-transparent px-1 text-[13px] text-ink outline-none placeholder:text-ink-3"
          />
          <span className="min-w-12 text-right font-mono text-[11px] text-ink-3 tabular-nums">
            {find.query.trim() ? `${find.activeMatch}/${find.matchCount}` : ''}
          </span>
          <button
            type="button"
            aria-label="Previous match"
            title="Previous match"
            disabled={find.matchCount === 0}
            onClick={find.findPrevious}
            className="grid size-8 shrink-0 cursor-pointer place-items-center rounded-full text-ink-3 transition-colors duration-150 hover:bg-surface-2 hover:text-ink disabled:cursor-default disabled:opacity-40"
          >
            <ChevronUp size={15} />
          </button>
          <button
            type="button"
            aria-label="Next match"
            title="Next match"
            disabled={find.matchCount === 0}
            onClick={find.findNext}
            className="grid size-8 shrink-0 cursor-pointer place-items-center rounded-full text-ink-3 transition-colors duration-150 hover:bg-surface-2 hover:text-ink disabled:cursor-default disabled:opacity-40"
          >
            <ChevronDown size={15} />
          </button>
          <button
            type="button"
            aria-label="Close find"
            title="Close find"
            onClick={find.closeFind}
            className="grid size-8 shrink-0 cursor-pointer place-items-center rounded-full text-ink-3 transition-colors duration-150 hover:bg-surface-2 hover:text-ink"
          >
            <X size={15} />
          </button>
        </motion.div>
      ) : null}
    </AnimatePresence>
  )
}
