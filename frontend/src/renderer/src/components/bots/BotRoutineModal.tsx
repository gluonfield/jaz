import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useEffect, type ReactNode } from 'react'
import { loopTiming } from '@/components/loops/schedule'
import { Button } from '@/components/ui/Button'
import { Modal } from '@/components/ui/Modal'
import { useToast } from '@/components/ui/toast'
import { apiUrl } from '@/lib/api/client'
import { botRoutinesQuery, deleteLoop, loopDetailQuery, runLoopNow, updateLoop } from '@/lib/api/loops'
import type { Bot, LoopRun } from '@/lib/api/types'
import { writeClipboard } from '@/lib/clipboard'
import { hasTime, messageTime } from '@/lib/format/time'
import { keys } from '@/lib/query/keys'

const RUN_STATUS: Record<LoopRun['status'], [string, string]> = {
  starting: ['Queued', 'text-running'],
  running: ['Running', 'text-running'],
  ok: ['Ran', 'text-ink-2'],
  error: ['Failed', 'text-danger'],
  cancelled: ['Stopped', 'text-ink-3'],
  skipped: ['Skipped', 'text-ink-3'],
}

// One of a bot's routines: what it does, when and where it runs, how its
// recent runs went, and the actions on it.
export function BotRoutineModal({ bot, loopId, onClose }: { bot: Bot; loopId: string; onClose: () => void }) {
  const queryClient = useQueryClient()
  const toast = useToast()
  const routines = useQuery(botRoutinesQuery(bot.id))
  const loop = routines.data?.find((item) => item.id === loopId)
  const runs = useQuery(loopDetailQuery(loopId)).data?.runs ?? []
  const act = useMutation({
    mutationFn: (action: 'toggle' | 'run' | 'delete'): Promise<unknown> =>
      action === 'toggle'
        ? updateLoop(loopId, { status: loop?.status === 'paused' ? 'active' : 'paused' })
        : action === 'run'
          ? runLoopNow(loopId)
          : deleteLoop(loopId),
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: keys.loops })
      void queryClient.invalidateQueries({ queryKey: keys.bots })
    },
    onError: (error) => toast(`Couldn't update ${loop?.name ?? 'the routine'}: ${error.message}`, 'danger'),
  })
  const gone = routines.isSuccess && !loop
  useEffect(() => {
    if (gone) onClose()
  }, [gone, onClose])
  if (!loop) return null
  const paused = loop.status === 'paused'

  return (
    <Modal
      open
      onClose={onClose}
      title={loop.name}
      description={paused ? 'Paused' : undefined}
      footer={
        <>
          <Button
            variant="danger"
            disabled={act.isPending}
            onClick={() => {
              if (window.confirm(`Delete ${loop.name}?`)) act.mutate('delete')
            }}
          >
            Delete
          </Button>
          <div className="flex gap-2">
            <Button variant="secondary" disabled={act.isPending} onClick={() => act.mutate('toggle')}>
              {paused ? 'Resume' : 'Pause'}
            </Button>
            <Button variant="primary" disabled={act.isPending} onClick={() => act.mutate('run')}>
              Run now
            </Button>
          </div>
        </>
      }
    >
      <div className="flex flex-col gap-4">
        <p className="max-h-48 overflow-y-auto rounded-lg bg-surface px-3 py-2.5 text-[13px] leading-relaxed whitespace-pre-wrap text-ink">{loop.prompt}</p>
        <dl className="flex flex-col">
          <Fact label="Runs">
            {loopTiming(loop) || loop.schedule.expr}
            {!paused && hasTime(loop.next_run_at) ? <span className="text-ink-3"> · next {messageTime(loop.next_run_at)}</span> : null}
          </Fact>
          <Fact label="If missed">{loop.when_missed === 'catch_up' ? 'Catches up with one run' : 'Skips it'}</Fact>
          <Fact label="Works in">{loop.thread === 'main' ? `${bot.name}'s chat` : 'A side chat of its own'}</Fact>
          {loop.trigger?.kind === 'webhook' ? (
            <Fact label="Webhook">
              <button type="button" className="cursor-pointer text-primary hover:underline" onClick={() => void writeClipboard(apiUrl(`/v1/hooks/${loop.id}`))}>
                Copy URL
              </button>
            </Fact>
          ) : null}
        </dl>
        {runs.length ? (
          <section className="flex flex-col gap-1">
            <h3 className="text-[12px] font-medium text-ink-3">Recent runs</h3>
            {runs.slice(0, 8).map((run) => (
              <RunRow key={run.id} run={run} bot={bot} onOpen={onClose} />
            ))}
          </section>
        ) : null}
      </div>
    </Modal>
  )
}

function Fact({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex gap-3 py-1 text-[13px]">
      <dt className="w-20 shrink-0 text-ink-3">{label}</dt>
      <dd className="min-w-0 text-ink">{children}</dd>
    </div>
  )
}

// A run that worked in a side chat opens it; one in the bot's own chat is
// already on screen.
function RunRow({ run, bot, onOpen }: { run: LoopRun; bot: Bot; onOpen: () => void }) {
  const [label, tone] = RUN_STATUS[run.status]
  const at = [run.finished_at, run.started_at, run.scheduled_for].find(hasTime)
  const line = (
    <>
      <span className={`w-16 shrink-0 ${tone}`}>{label}</span>
      <span className="shrink-0 text-ink-3 tabular-nums">{at ? messageTime(at) : ''}</span>
      {run.error ? <span className="min-w-0 truncate text-danger">{run.error}</span> : null}
    </>
  )
  const className = '-mx-2 flex h-7 items-center gap-2 rounded-md px-2 text-[12px]'
  return run.thread_id && run.thread_id !== bot.id ? (
    <Link to="/sessions/$sessionId" params={{ sessionId: run.thread_id }} onClick={onOpen} className={`${className} hover:bg-list-hover`}>
      {line}
    </Link>
  ) : (
    <div className={className}>{line}</div>
  )
}
