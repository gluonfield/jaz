import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, createFileRoute, useNavigate } from '@tanstack/react-router'
import { ArrowLeft, ChevronRight, Pencil, Play, Trash2 } from 'lucide-react'
import { type Variants, motion, useReducedMotion } from 'motion/react'
import { type ReactNode, useState } from 'react'
import { LoopModal } from '@/components/loops/LoopModal'
import { reasoningEffortLabel } from '@/components/loops/ReasoningEffortSelect'
import { describeSchedule, draftFromLoop } from '@/components/loops/schedule'
import { Button } from '@/components/ui/Button'
import { EmptyState } from '@/components/ui/EmptyState'
import { IconButton } from '@/components/ui/IconButton'
import { SkeletonRows } from '@/components/ui/Skeleton'
import { useToast } from '@/components/ui/toast'
import { agentLabel } from '@/lib/agentLabel'
import { deleteLoop, loopDetailQuery, runLoopNow } from '@/lib/api/loops'
import type { Loop, LoopRun } from '@/lib/api/types'
import { fullTime, hasTime, relativeTime } from '@/lib/format/time'
import { keys } from '@/lib/query/keys'

export const Route = createFileRoute('/loops/$loopId')({
  component: LoopDetailPage,
})

function LoopDetailPage() {
  const { loopId } = Route.useParams()
  const detail = useQuery(loopDetailQuery(loopId))

  if (detail.isPending) {
    return (
      <div className="mx-auto max-w-[620px] px-10 pb-12 pt-6">
        <SkeletonRows count={6} />
      </div>
    )
  }
  if (detail.isError) {
    return (
      <div className="mx-auto max-w-[620px] px-10 pb-12">
        <EmptyState title="Couldn't load this loop">
          <p>{detail.error.message}</p>
        </EmptyState>
      </div>
    )
  }
  return <LoopDetail loop={detail.data.loop} runs={detail.data.runs} />
}

function LoopDetail({ loop, runs }: { loop: Loop; runs: LoopRun[] }) {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const toast = useToast()
  const reduce = useReducedMotion()
  const [editing, setEditing] = useState(false)

  const invalidate = () => {
    queryClient.invalidateQueries({ queryKey: keys.loopDetail(loop.id) })
    queryClient.invalidateQueries({ queryKey: keys.loops })
  }

  const run = useMutation({
    mutationFn: () => runLoopNow(loop.id),
    onSuccess: () => {
      toast('Loop run started')
      invalidate()
    },
    onError: (error) => toast(`Couldn't run: ${(error as Error).message}`, 'danger'),
  })

  const remove = useMutation({
    mutationFn: () => deleteLoop(loop.id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: keys.loops })
      navigate({ to: '/loops' })
    },
    onError: (error) => toast(`Couldn't delete: ${(error as Error).message}`, 'danger'),
  })

  const onDelete = () => {
    if (confirm(`Delete loop "${loop.name}"? Its run history is kept but it stops running.`)) {
      remove.mutate()
    }
  }

  const paused = loop.status === 'paused'
  const isAcp = loop.runtime === 'acp'
  const summary = describeSchedule(draftFromLoop(loop.schedule?.expr ?? '', paused))
  const nextRun =
    !paused && hasTime(loop.next_run_at)
      ? new Date(loop.next_run_at as string).toLocaleDateString(undefined, { month: 'short', day: 'numeric' })
      : ''

  const container: Variants = {
    hidden: {},
    show: { transition: { staggerChildren: reduce ? 0 : 0.05, delayChildren: reduce ? 0 : 0.02 } },
  }
  const item: Variants = {
    hidden: reduce ? { opacity: 0 } : { opacity: 0, y: 8 },
    show: { opacity: 1, y: 0, transition: { duration: 0.28, ease: [0.22, 1, 0.36, 1] } },
  }

  return (
    <motion.div
      className="mx-auto max-w-[620px] px-10 pb-20 pt-6"
      variants={container}
      initial="hidden"
      animate="show"
    >
      <motion.div variants={item} className="pb-3">
        <Link
          to="/loops"
          className="inline-flex items-center gap-1.5 text-[12px] text-ink-3 transition-colors duration-150 hover:text-ink"
        >
          <ArrowLeft size={14} />
          All loops
        </Link>
      </motion.div>

      <motion.header variants={item} className="flex items-start justify-between gap-4">
        <div className="min-w-0">
          <div className="flex items-center gap-2.5">
            <h1 className="truncate text-[22px] font-semibold tracking-[-0.01em] text-ink">{loop.name}</h1>
            <StatusPill loop={loop} />
          </div>
          <p className="mt-1 text-[13px] text-ink-2">
            {summary}
            {nextRun ? <span className="text-ink-3"> · next {nextRun}</span> : null}
          </p>
        </div>
        <div className="flex shrink-0 items-center gap-1.5">
          <Action onClick={() => run.mutate()} disabled={run.isPending}>
            <Play size={13} />
            {run.isPending ? 'Starting…' : 'Run now'}
          </Action>
          <Action onClick={() => setEditing(true)}>
            <Pencil size={13} />
            Edit
          </Action>
          <IconButton
            variant="danger"
            size="md"
            aria-label="Delete loop"
            title="Delete loop"
            onClick={onDelete}
          >
            <Trash2 size={15} />
          </IconButton>
        </div>
      </motion.header>

      <motion.div variants={item} className="mt-6 whitespace-pre-wrap rounded-card bg-surface px-4 py-3.5 text-[13.5px] leading-relaxed text-ink">
        {loop.prompt}
      </motion.div>

      <motion.dl variants={item} className="mt-5 flex flex-wrap gap-x-10 gap-y-3">
        <Fact label="Agent" value={isAcp ? agentLabel(loop.acp_agent) : 'Native'} />
        <Fact label="Reasoning effort" value={reasoningEffortLabel(loop.reasoning_effort)} />
        <Fact label="Folder" value={loop.directory?.trim() || 'workspace'} mono />
      </motion.dl>

      <motion.section variants={item} className="mt-10">
        <div className="flex items-baseline justify-between border-b border-border pb-2">
          <h2 className="text-[13px] font-semibold text-ink">Last runs</h2>
          {runs.length > 0 ? (
            <span className="text-[12px] tabular-nums text-ink-3">{runs.length}</span>
          ) : null}
        </div>
        {runs.length === 0 ? (
          <p className="pt-3 text-[13px] text-ink-3">No runs yet — use Run now to try it.</p>
        ) : (
          <div className="pt-1">
            {runs.map((item) => (
              <RunRow key={item.id} run={item} />
            ))}
          </div>
        )}
      </motion.section>

      <LoopModal open={editing} onClose={() => setEditing(false)} loop={loop} />
    </motion.div>
  )
}

function Action({
  onClick,
  disabled,
  children,
}: {
  onClick: () => void
  disabled?: boolean
  children: ReactNode
}) {
  return (
    <Button variant="secondary" size="md" disabled={disabled} onClick={onClick}>
      {children}
    </Button>
  )
}

function Fact({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div>
      <dt className="text-[11px] font-medium text-ink-3">{label}</dt>
      <dd className={`mt-0.5 text-[13px] text-ink ${mono ? 'font-mono' : ''}`}>{value}</dd>
    </div>
  )
}

const PILL = {
  failed: 'bg-danger-soft text-danger',
  running: 'bg-running/15 text-running',
  paused: 'bg-surface-2 text-ink-3',
  active: 'bg-primary-soft text-primary-strong',
} as const

function StatusPill({ loop }: { loop: Loop }) {
  const tone =
    loop.last_run_status === 'error'
      ? 'failed'
      : loop.last_run_status === 'running' || loop.last_run_status === 'starting'
        ? 'running'
        : loop.status === 'paused'
          ? 'paused'
          : 'active'
  const label = tone === 'failed' ? 'failed' : tone
  return (
    <span className={`shrink-0 rounded-full px-2 py-0.5 text-[11px] font-medium ${PILL[tone]}`}>{label}</span>
  )
}

function RunRow({ run }: { run: LoopRun }) {
  // Go marshals an unset time.Time as "0001-01-01T00:00:00Z" (omitempty is a
  // no-op for structs), so a still-starting run's zero started_at is truthy and
  // would render as "Dec 31"; pick the first field that carries a real time.
  const when = [run.started_at, run.scheduled_for, run.created_at].find(hasTime) ?? run.created_at
  const body = (
    <>
      <RunStatusDot status={run.status} />
      <span className="min-w-0 flex-1 truncate text-[13px] text-ink-2" title={fullTime(when)}>
        {relativeTime(when)}
        {run.error ? <span className="text-ink-3"> · {run.error}</span> : null}
      </span>
      <span className="shrink-0 text-[11px] tabular-nums text-ink-3">{formatDuration(run) || run.status}</span>
    </>
  )

  if (run.thread_id) {
    return (
      <Link
        to="/sessions/$sessionId"
        params={{ sessionId: run.thread_id }}
        className="group flex items-center gap-2.5 rounded-card px-3 py-2 transition-colors duration-150 hover:bg-surface-2"
      >
        {body}
        <ChevronRight size={14} className="-mr-0.5 shrink-0 text-ink-3 opacity-0 transition-opacity group-hover:opacity-100" />
      </Link>
    )
  }
  return <div className="flex items-center gap-2.5 px-2 py-2">{body}</div>
}

function RunStatusDot({ status }: { status: LoopRun['status'] }) {
  const color =
    status === 'ok'
      ? 'bg-ok'
      : status === 'error'
        ? 'bg-danger'
        : status === 'running' || status === 'starting'
          ? 'bg-running animate-pulse'
          : 'bg-ink-3/50'
  return <span title={status} className={`size-1.5 shrink-0 rounded-full ${color}`} />
}

function formatDuration(run: LoopRun): string {
  if (!run.started_at || !run.finished_at) return ''
  const ms = new Date(run.finished_at).getTime() - new Date(run.started_at).getTime()
  if (!Number.isFinite(ms) || ms < 0) return ''
  const seconds = Math.round(ms / 1000)
  if (seconds < 60) return `${seconds}s`
  const minutes = Math.floor(seconds / 60)
  return `${minutes}m ${seconds % 60}s`
}
