import { useQuery } from '@tanstack/react-query'
import { Link, createFileRoute } from '@tanstack/react-router'
import { ChevronRight, Plus } from 'lucide-react'
import { type Variants, motion, useReducedMotion } from 'motion/react'
import { useState } from 'react'
import { LoopModal } from '@/components/loops/LoopModal'
import { compactSchedule } from '@/components/loops/schedule'
import { EmptyState } from '@/components/ui/EmptyState'
import { SkeletonRows } from '@/components/ui/Skeleton'
import { agentLabel } from '@/lib/agentLabel'
import { loopsQuery } from '@/lib/api/loops'
import type { Loop } from '@/lib/api/types'
import { hasTime, relativeTime } from '@/lib/format/time'

export const Route = createFileRoute('/loops/')({
  component: LoopsPage,
})

function LoopsPage() {
  const loops = useQuery(loopsQuery)
  const [creating, setCreating] = useState(false)
  const reduce = useReducedMotion()

  const container: Variants = {
    hidden: {},
    show: { transition: { staggerChildren: reduce ? 0 : 0.04, delayChildren: reduce ? 0 : 0.03 } },
  }
  const row: Variants = {
    hidden: reduce ? { opacity: 0 } : { opacity: 0, y: 8 },
    show: { opacity: 1, y: 0, transition: { duration: 0.26, ease: [0.22, 1, 0.36, 1] } },
  }

  return (
    <div className="mx-auto max-w-[620px] px-10 pb-16 pt-6">
      <header className="flex items-end justify-between pb-6">
        <div>
          <h1 className="text-[22px] font-semibold tracking-[-0.01em] text-ink">Loops</h1>
          <p className="mt-1 text-[13px] text-ink-3">Prompts that run on a schedule.</p>
        </div>
        <motion.button
          type="button"
          onClick={() => setCreating(true)}
          whileTap={{ scale: 0.97 }}
          className="inline-flex h-9 items-center gap-1.5 rounded-control bg-primary px-3.5 text-[13px] font-medium text-on-primary transition-colors duration-150 hover:bg-primary-strong"
        >
          <Plus size={15} />
          New loop
        </motion.button>
      </header>

      {loops.isPending ? (
        <SkeletonRows count={5} />
      ) : loops.isError ? (
        <EmptyState title="Couldn't load loops">
          <p>{loops.error.message}</p>
        </EmptyState>
      ) : loops.data.length === 0 ? (
        <button
          type="button"
          onClick={() => setCreating(true)}
          className="flex w-full flex-col items-center gap-2 rounded-card border border-dashed border-border px-6 py-12 text-center transition-colors duration-150 hover:border-primary/50 hover:bg-surface"
        >
          <span className="grid size-10 place-items-center rounded-full bg-surface-2 text-ink-3">
            <Plus size={18} />
          </span>
          <span className="text-[14px] font-medium text-ink">Create your first loop</span>
          <span className="max-w-[42ch] text-[13px] text-ink-3">
            Run a prompt on a schedule — each run gets its own thread you can open later.
          </span>
        </button>
      ) : (
        <motion.div variants={container} initial="hidden" animate="show" className="-mx-2">
          {loops.data.map((loop) => (
            <motion.div key={loop.id} variants={row}>
              <LoopRow loop={loop} />
            </motion.div>
          ))}
        </motion.div>
      )}

      <LoopModal open={creating} onClose={() => setCreating(false)} />
    </div>
  )
}

function LoopRow({ loop }: { loop: Loop }) {
  const agent = loop.runtime === 'acp' ? agentLabel(loop.acp_agent) : 'Native'
  return (
    <Link
      to="/loops/$loopId"
      params={{ loopId: loop.id }}
      className="group flex items-center gap-3 rounded-control px-2 py-2.5 transition-colors duration-150 hover:bg-surface"
    >
      <StatusDot loop={loop} />
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2">
          <span className="truncate text-[13.5px] font-medium text-ink">{loop.name}</span>
          {loop.last_run_status === 'error' ? (
            <span className="shrink-0 rounded-full bg-danger-soft px-1.5 py-px text-[10px] font-medium text-danger">
              failed
            </span>
          ) : loop.status === 'paused' ? (
            <span className="shrink-0 rounded-full bg-surface-2 px-1.5 py-px text-[10px] font-medium text-ink-3">
              paused
            </span>
          ) : null}
        </div>
        <p className="mt-0.5 truncate text-[12px] text-ink-3">
          {compactSchedule(loop.schedule?.expr ?? '', loop.status === 'paused')} · {agent}
        </p>
      </div>
      {hasTime(loop.last_run_at) ? (
        <span className="shrink-0 text-[11px] tabular-nums text-ink-3">ran {relativeTime(loop.last_run_at as string)}</span>
      ) : null}
      <ChevronRight size={15} className="-mr-0.5 shrink-0 text-ink-3 opacity-0 transition-opacity group-hover:opacity-100" />
    </Link>
  )
}

function StatusDot({ loop }: { loop: Loop }) {
  const color =
    loop.last_run_status === 'error'
      ? 'bg-danger'
      : loop.last_run_status === 'running' || loop.last_run_status === 'starting'
        ? 'bg-running animate-pulse'
        : loop.status === 'paused'
          ? 'bg-ink-3/40'
          : 'bg-primary'
  return <span className={`size-2 shrink-0 rounded-full ${color}`} />
}
