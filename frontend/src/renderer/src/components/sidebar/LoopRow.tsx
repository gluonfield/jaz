import { Link } from '@tanstack/react-router'
import { compactSchedule } from '@/components/loops/schedule'
import type { Loop } from '@/lib/api/types'

function LoopStatusDot({ loop }: { loop: Loop }) {
  if (loop.last_run_status === 'running' || loop.last_run_status === 'starting') {
    return <span title="Running" className="size-1.5 shrink-0 animate-pulse rounded-full bg-running" />
  }
  if (loop.last_run_status === 'error') {
    return (
      <span
        title={loop.last_error ? `Last run failed: ${loop.last_error}` : 'Last run failed'}
        className="size-1.5 shrink-0 rounded-full bg-danger"
      />
    )
  }
  if (loop.status === 'paused') {
    return <span title="Paused" className="size-1.5 shrink-0 rounded-full bg-ink-3/50" />
  }
  return <span title="Active" className="size-1.5 shrink-0 rounded-full bg-primary" />
}

export function LoopRow({ loop }: { loop: Loop }) {
  return (
    <Link
      to="/loops/$loopId"
      params={{ loopId: loop.id }}
      className="group flex items-center gap-2 rounded-control px-2 py-1.5 text-[13px] text-ink-2 transition-colors duration-150 hover:bg-surface-2 hover:text-ink"
      activeProps={{ className: 'bg-primary-soft! text-ink! font-medium' }}
    >
      <LoopStatusDot loop={loop} />
      <span className="min-w-0 flex-1 truncate" title={loop.name}>
        {loop.name}
      </span>
      <span className="shrink-0 text-[11px] tabular-nums text-ink-3">
        {compactSchedule(loop.schedule?.expr ?? '', loop.status === 'paused')}
      </span>
    </Link>
  )
}
