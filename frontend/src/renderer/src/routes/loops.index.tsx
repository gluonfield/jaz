import { useQuery } from '@tanstack/react-query'
import { Link, createFileRoute } from '@tanstack/react-router'
import { ChevronRight, Plus } from 'lucide-react'
import { useState } from 'react'
import { LoopModal } from '@/components/loops/LoopModal'
import { compactSchedule } from '@/components/loops/schedule'
import { Button } from '@/components/ui/Button'
import { DashedCta } from '@/components/ui/DashedCta'
import { EmptyState } from '@/components/ui/EmptyState'
import { SkeletonRows } from '@/components/ui/Skeleton'
import { agentLabel } from '@/lib/agentLabel'
import { loopsQuery } from '@/lib/api/loops'
import type { Loop } from '@/lib/api/types'
import { hasTime, relativeTime, shortDate } from '@/lib/format/time'

export const Route = createFileRoute('/loops/')({
  component: LoopsPage,
})

function LoopsPage() {
  const loops = useQuery(loopsQuery)
  const [creating, setCreating] = useState(false)

  return (
    <div className="mx-auto max-w-[820px] px-10 pb-16 pt-6">
      <header className="flex items-end justify-between pb-6">
        <div>
          <h1 className="text-[22px] font-semibold tracking-[-0.01em] text-ink">Loops</h1>
          <p className="mt-1 text-[13px] text-ink-3">Prompts that run on a schedule.</p>
        </div>
        <Button variant="primary" size="lg" onClick={() => setCreating(true)}>
          <Plus size={15} />
          New loop
        </Button>
      </header>

      {loops.isPending ? (
        <SkeletonRows count={5} />
      ) : loops.isError ? (
        <EmptyState title="Couldn't load loops">
          <p>{loops.error.message}</p>
        </EmptyState>
      ) : loops.data.length === 0 ? (
        <DashedCta
          onClick={() => setCreating(true)}
          title="Create your first loop"
          subtitle="Run a prompt on a schedule — each run gets its own thread you can open later."
        />
      ) : (
        <div className="-mx-2">
          {loops.data.map((loop) => (
            <LoopRow key={loop.id} loop={loop} />
          ))}
          <DotLegend />
        </div>
      )}

      <LoopModal open={creating} onClose={() => setCreating(false)} />
    </div>
  )
}

function LoopRow({ loop }: { loop: Loop }) {
  const agent = agentLabel(loop.acp_agent || 'jaz')
  const paused = loop.status === 'paused'
  const nextRun = !paused && hasTime(loop.next_run_at) ? shortDate(loop.next_run_at) : ''
  return (
    <Link
      to="/loops/$loopId"
      params={{ loopId: loop.id }}
      className="group flex items-center gap-3.5 rounded-card px-3 py-3 transition-colors duration-150 hover:bg-surface"
    >
      <StatusDot loop={loop} />
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2">
          <span className="truncate text-[14px] font-medium text-ink">{loop.name}</span>
          {loop.last_run_status === 'error' ? (
            <span className="shrink-0 rounded-full bg-danger-soft px-1.5 py-px text-[10px] font-medium text-danger">
              failed
            </span>
          ) : paused ? (
            <span className="shrink-0 rounded-full bg-surface-2 px-1.5 py-px text-[10px] font-medium text-ink-3">
              paused
            </span>
          ) : null}
        </div>
        <p className="mt-0.5 truncate text-[12.5px] text-ink-3">{loop.prompt}</p>
      </div>
      <div className="flex shrink-0 flex-col items-end gap-0.5 text-right">
        <span className="text-[12px] text-ink-2">
          {compactSchedule(loop.schedule?.expr ?? '', paused)} · {agent}
        </span>
        <span className="text-[11px] tabular-nums text-ink-3">
          {hasTime(loop.last_run_at)
            ? `ran ${relativeTime(loop.last_run_at)}`
            : nextRun
              ? `next ${nextRun}`
              : ''}
        </span>
      </div>
      <ChevronRight size={15} className="-mr-0.5 shrink-0 text-ink-3 opacity-0 transition-opacity group-hover:opacity-100" />
    </Link>
  )
}

const LEGEND: Array<{ label: string; dot: string }> = [
  { label: 'active', dot: 'bg-primary' },
  { label: 'running', dot: 'bg-running animate-pulse' },
  { label: 'failed', dot: 'bg-danger' },
  { label: 'paused', dot: 'bg-ink-3/40' },
]

function DotLegend() {
  return (
    <div className="mt-4 flex items-center gap-4 px-3">
      {LEGEND.map(({ label, dot }) => (
        <span key={label} className="flex items-center gap-1.5 text-[11px] text-ink-3">
          <span className={`size-1.5 shrink-0 rounded-full ${dot}`} />
          {label}
        </span>
      ))}
    </div>
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
