import { useQuery } from '@tanstack/react-query'
import { Briefcase, CalendarDays, Clock3, type LucideIcon, Repeat, Sun, Zap } from 'lucide-react'
import { CADENCES, type Cadence, loopCadence, loopWhen } from '@/components/loops/schedule'
import { botRoutinesQuery, loopTone } from '@/lib/api/loops'
import type { Bot, Loop } from '@/lib/api/types'
import { hasTime, messageTime } from '@/lib/format/time'

const CADENCE_ICON: Record<Cadence, LucideIcon> = {
  'Through the day': Repeat,
  Daily: Sun,
  Weekdays: Briefcase,
  Weekly: CalendarDays,
  'When something arrives': Zap,
  Other: Clock3,
}

export function BotRoutines({ bot, onOpen }: { bot: Bot; onOpen: (id: string) => void }) {
  const routines = useQuery(botRoutinesQuery(bot.id))
  if (routines.isPending) return null
  if (!routines.data?.length) return <p className="text-[13px] text-ink-3">Ask {bot.name} to schedule something.</p>
  return (
    <div className="flex flex-col gap-4">
      {CADENCES.map((cadence) => {
        const loops = routines.data.filter((loop) => loopCadence(loop) === cadence)
        return loops.length ? (
          <section key={cadence} className="flex flex-col gap-1">
            <h3 className="text-[13px] font-semibold text-ink">{cadence}</h3>
            {loops.map((loop) => (
              <RoutineRow key={loop.id} loop={loop} Icon={CADENCE_ICON[cadence]} onOpen={() => onOpen(loop.id)} />
            ))}
          </section>
        ) : null
      })}
    </div>
  )
}

function RoutineRow({ loop, Icon, onOpen }: { loop: Loop; Icon: LucideIcon; onOpen: () => void }) {
  const paused = loop.status === 'paused'
  const tone = loopTone(loop.last_run_status, loop.status)
  const when = loopWhen(loop) || (hasTime(loop.next_run_at) ? `Next ${messageTime(loop.next_run_at)}` : '')
  return (
    <button
      type="button"
      onClick={onOpen}
      className="-mx-2 flex cursor-pointer items-center gap-3 rounded-lg px-2 py-1.5 text-left transition-colors duration-150 hover:bg-list-hover"
    >
      <span className="grid size-9 shrink-0 place-items-center rounded-full bg-surface-2 text-ink-2">
        <Icon size={16} aria-hidden />
      </span>
      <span className="min-w-0 flex-1">
        <span className={`block truncate text-[13px] font-medium ${paused ? 'text-ink-3' : 'text-ink'}`}>{loop.name}</span>
        <span className="block truncate text-[12px] text-ink-3">
          {paused ? 'Paused · ' : null}
          {tone === 'running' ? <span className="text-running">{loop.last_run_status === 'starting' ? 'Queued' : 'Running now'} · </span> : null}
          {tone === 'failed' ? <span className="text-danger">Last run failed · </span> : null}
          {when}
        </span>
      </span>
    </button>
  )
}
