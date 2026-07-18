import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, createFileRoute, useNavigate } from '@tanstack/react-router'
import { ArrowLeft, ChevronRight, MoreHorizontal, Pencil, Play, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { LoopBoardAssignments } from '@/components/loops/LoopBoardAssignments'
import { LoopModal } from '@/components/loops/LoopModal'
import { describeSchedule, draftFromLoop } from '@/components/loops/schedule'
import { MentionText } from '@/components/session/mentions'
import { EmptyState } from '@/components/ui/EmptyState'
import { IconButton } from '@/components/ui/IconButton'
import { MenuRow, Popover } from '@/components/ui/Popover'
import { SkeletonRows } from '@/components/ui/Skeleton'
import { useToast } from '@/components/ui/toast'
import { deleteLoop, loopDetailQuery, loopTone, runLoopNow, type LoopTone } from '@/lib/api/loops'
import type { Loop, LoopRun } from '@/lib/api/types'
import { fullTime, hasTime, relativeTime, shortDate } from '@/lib/format/time'
import { keys } from '@/lib/query/keys'

export const Route = createFileRoute('/loops/$loopId')({
  component: LoopDetailPage,
})

function LoopDetailPage() {
  const { loopId } = Route.useParams()
  const detail = useQuery(loopDetailQuery(loopId))

  if (detail.isPending) {
    return (
      <div className="mx-auto max-w-[820px] px-10 pb-12 pt-6">
        <SkeletonRows count={6} />
      </div>
    )
  }
  if (detail.isError) {
    return (
      <div className="mx-auto max-w-[820px] px-10 pb-12">
        <EmptyState title="Couldn't load this loop">
          <p>{detail.error.message}</p>
        </EmptyState>
      </div>
    )
  }
  return (
    <LoopDetail
      loop={detail.data.loop}
      runs={detail.data.runs}
      boardIds={detail.data.boardIds}
    />
  )
}

function LoopDetail({
  loop,
  runs,
  boardIds,
}: {
  loop: Loop
  runs: LoopRun[]
  boardIds: string[]
}) {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const toast = useToast()
  const [editing, setEditing] = useState(false)
  const [actionsOpen, setActionsOpen] = useState(false)

  const invalidate = () => {
    queryClient.invalidateQueries({ queryKey: keys.loopDetail(loop.id) })
    queryClient.invalidateQueries({ queryKey: keys.loops })
  }

  const run = useMutation<LoopRun, Error, void>({
    mutationFn: () => runLoopNow(loop.id),
    onSuccess: () => {
      toast('Loop run started')
      invalidate()
    },
    onError: (error) => toast(`Couldn't run: ${error.message}`, 'danger'),
  })

  const remove = useMutation<{ ok: boolean }, Error, void>({
    mutationFn: () => deleteLoop(loop.id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: keys.loops })
      navigate({ to: '/loops' })
    },
    onError: (error) => toast(`Couldn't delete: ${error.message}`, 'danger'),
  })

  const onDelete = () => {
    setActionsOpen(false)
    if (confirm(`Delete loop "${loop.name}"? Its run history is kept but it stops running.`)) {
      remove.mutate()
    }
  }

  const paused = loop.status === 'paused'
  const summary = describeSchedule(draftFromLoop(loop.schedule?.expr ?? '', paused))
  const nextRun = !paused && hasTime(loop.next_run_at) ? shortDate(loop.next_run_at) : ''

  return (
    <div className="mx-auto max-w-[820px] px-10 pb-20 pt-6">
      <div className="pb-3">
        <Link
          to="/loops"
          className="inline-flex items-center gap-1.5 text-[12px] text-ink-3 transition-colors duration-150 hover:text-ink"
        >
          <ArrowLeft size={14} />
          All loops
        </Link>
      </div>

      <header className="flex items-start justify-between gap-6">
        <div className="min-w-0 flex-1">
          <div className="flex min-w-0 items-center gap-2.5">
            <h1 className="min-w-0 truncate text-[24px] font-semibold tracking-[-0.01em] text-ink">{loop.name}</h1>
            <StatusPill loop={loop} />
          </div>
          <p className="mt-1 text-[13px] text-ink-2">
            {summary}
            {nextRun ? <span className="text-ink-3"> · next {nextRun}</span> : null}
          </p>
        </div>
        <div className="flex shrink-0 items-center gap-1.5">
          <IconButton
            variant="primary"
            size="md"
            aria-label={run.isPending ? 'Starting loop' : 'Run loop now'}
            title={run.isPending ? 'Starting loop' : 'Run loop now'}
            disabled={run.isPending}
            onClick={() => run.mutate()}
          >
            <Play size={14} />
          </IconButton>
          <Popover
            open={actionsOpen}
            onClose={() => setActionsOpen(false)}
            placement="below"
            align="end"
            trigger={
              <IconButton
                variant="ghost"
                size="md"
                aria-label="Loop actions"
                title="Loop actions"
                onClick={() => setActionsOpen((open) => !open)}
              >
                <MoreHorizontal size={15} />
              </IconButton>
            }
          >
            <MenuRow
              onClick={() => {
                setActionsOpen(false)
                setEditing(true)
              }}
            >
              <span className="flex items-center gap-2">
                <Pencil size={13} />
                Edit
              </span>
            </MenuRow>
            <MenuRow disabled={remove.isPending} onClick={onDelete}>
              <span className="flex items-center gap-2 text-danger">
                <Trash2 size={13} />
                Delete
              </span>
            </MenuRow>
          </Popover>
        </div>
      </header>

      <div className="mt-5 whitespace-pre-wrap rounded-card bg-surface px-3.5 py-2.5 text-[12.5px] leading-relaxed text-ink-2">
        <MentionText text={loop.prompt} />
      </div>

      <LoopBoardAssignments loop={loop} boardIds={boardIds} />

      <section className="mt-10">
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
      </section>

      <LoopModal open={editing} onClose={() => setEditing(false)} loop={loop} boardIds={boardIds} />
    </div>
  )
}

const PILL: Record<LoopTone, string> = {
  failed: 'bg-danger-soft text-danger',
  running: 'bg-running/15 text-running',
  paused: 'bg-surface-2 text-ink-3',
  active: 'bg-primary-soft text-primary-strong',
}

function StatusPill({ loop }: { loop: Loop }) {
  const tone = loopTone(loop.last_run_status, loop.status)
  return (
    <span className={`shrink-0 rounded-full px-2 py-0.5 text-[11px] font-medium ${PILL[tone]}`}>{tone}</span>
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
          : 'bg-ink-3/40'
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
