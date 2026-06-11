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
import { hasTime, relativeTime } from '@/lib/format/time'

export const Route = createFileRoute('/loops/')({
  component: LoopsPage,
})

function LoopsPage() {
  const loops = useQuery(loopsQuery)
  const [creating, setCreating] = useState(false)

  return (
    <div className="mx-auto max-w-[620px] px-10 pb-16 pt-6">
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
  const agent = loop.runtime === 'acp' ? agentLabel(loop.acp_agent) : 'Native'
  return (
    <Link
      to="/loops/$loopId"
      params={{ loopId: loop.id }}
      className="group flex items-center gap-3 rounded-card px-3 py-2.5 transition-colors duration-150 hover:bg-surface"
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
