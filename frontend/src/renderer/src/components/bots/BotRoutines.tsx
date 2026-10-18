import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link2, MoreHorizontal, Play, Power, PowerOff, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { loopWhen } from '@/components/loops/schedule'
import { IconButton } from '@/components/ui/IconButton'
import { MenuRow, Popover } from '@/components/ui/Popover'
import { useToast } from '@/components/ui/toast'
import { apiUrl } from '@/lib/api/client'
import { botRoutinesQuery, deleteLoop, loopTone, runLoopNow, updateLoop } from '@/lib/api/loops'
import type { Bot, Loop } from '@/lib/api/types'
import { writeClipboard } from '@/lib/clipboard'
import { hasTime, messageTime } from '@/lib/format/time'
import { keys } from '@/lib/query/keys'

export function BotRoutines({ bot }: { bot: Bot }) {
  const routines = useQuery(botRoutinesQuery(bot.id))
  return (
    <section className="flex flex-col gap-1">
      {routines.data?.length ? (
        routines.data.map((loop) => <RoutineRow key={loop.id} loop={loop} />)
      ) : routines.isPending ? null : (
        <p className="text-[13px] text-ink-3">Ask {bot.name} to schedule something.</p>
      )}
    </section>
  )
}

function RoutineRow({ loop }: { loop: Loop }) {
  const queryClient = useQueryClient()
  const toast = useToast()
  const [menuOpen, setMenuOpen] = useState(false)
  const paused = loop.status === 'paused'
  const invalidate = () => {
    queryClient.invalidateQueries({ queryKey: keys.loops })
    queryClient.invalidateQueries({ queryKey: keys.bots })
  }
  const act = useMutation({
    mutationFn: (action: 'toggle' | 'run' | 'delete'): Promise<unknown> =>
      action === 'toggle'
        ? updateLoop(loop.id, { status: paused ? 'active' : 'paused' })
        : action === 'run'
          ? runLoopNow(loop.id)
          : deleteLoop(loop.id),
    onSettled: invalidate,
    onError: (error) => toast(`Couldn't update ${loop.name}: ${error.message}`, 'danger'),
  })
  const menu = (action: () => void) => () => {
    setMenuOpen(false)
    action()
  }
  const next = paused ? 'Disabled' : hasTime(loop.next_run_at) ? `next ${messageTime(loop.next_run_at)}` : ''
  // The switch already says on or off; only a run in progress or a failed
  // one needs saying.
  const tone = loopTone(loop.last_run_status, loop.status)

  return (
    <div className="flex items-center gap-2.5 py-1">
      <div className="min-w-0 flex-1">
        <p className={`truncate text-[13px] ${paused ? 'text-ink-3' : 'text-ink'}`}>{loop.name}</p>
        <p className="truncate text-[12px] text-ink-3">
          {tone === 'running' ? (
            <span className="text-running">{loop.last_run_status === 'starting' ? 'Queued' : 'Running now'} · </span>
          ) : null}
          {tone === 'failed' ? <span className="text-danger">Last run failed · </span> : null}
          {[loopWhen(loop), next].filter(Boolean).join(' · ')}
        </p>
      </div>
      <Popover
        open={menuOpen}
        onClose={() => setMenuOpen(false)}
        placement="below"
        align="end"
        trigger={
          <IconButton size="xs" aria-label={`${loop.name} actions`} title="More" onClick={() => setMenuOpen((open) => !open)}>
            <MoreHorizontal size={14} />
          </IconButton>
        }
      >
        <MenuRow onClick={menu(() => act.mutate('run'))}>
          <span className="flex items-center gap-2">
            <Play size={13} />
            Run now
          </span>
        </MenuRow>
        <MenuRow onClick={menu(() => act.mutate('toggle'))}>
          <span className="flex items-center gap-2">
            {paused ? <Power size={13} /> : <PowerOff size={13} />}
            {paused ? 'Enable' : 'Disable'}
          </span>
        </MenuRow>
        {loop.trigger?.kind === 'webhook' ? (
          <MenuRow onClick={menu(() => void writeClipboard(apiUrl(`/v1/hooks/${loop.id}`)))}>
            <span className="flex items-center gap-2">
              <Link2 size={13} />
              Copy webhook URL
            </span>
          </MenuRow>
        ) : null}
        <MenuRow
          onClick={menu(() => {
            if (window.confirm(`Delete ${loop.name}?`)) act.mutate('delete')
          })}
        >
          <span className="flex items-center gap-2 text-danger">
            <Trash2 size={13} />
            Delete
          </span>
        </MenuRow>
      </Popover>
    </div>
  )
}
