import { Link } from '@tanstack/react-router'
import { compactSchedule } from '@/components/loops/schedule'
import { TONE_DOT } from '@/lib/api/loops'
import type { LoopCreatedEvent } from '@/lib/api/types'

function nextRunLabel(iso?: string): string {
  if (!iso) return ''
  const parsed = Date.parse(iso)
  if (Number.isNaN(parsed)) return ''
  const time = new Date(parsed).toLocaleTimeString([], {
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  })
  return `next ${time}`
}

export function LoopCreatedCard({ loop }: { loop: LoopCreatedEvent }) {
  const paused = loop.status === 'paused'
  const meta = [
    compactSchedule(loop.schedule ?? '', paused),
    nextRunLabel(loop.next_run_at),
    loop.agent || 'default agent',
  ]
    .map((part) => part.trim())
    .filter(Boolean)
    .join(' · ')
  return (
    <div className="flex w-fit min-w-[280px] max-w-[52ch] flex-col rounded-card bg-surface px-4 py-3.5">
      <div className="flex items-center justify-between gap-3">
        <span className="text-[12px] text-ink-3">Loop scheduled</span>
        <span className="inline-flex items-center gap-1.5 text-[12px] text-ink-3">
          <span className={`size-1.5 rounded-full ${TONE_DOT[paused ? 'paused' : 'active']}`} aria-hidden />
          {paused ? 'Paused' : 'Active'}
        </span>
      </div>
      <p className="mt-1 text-[15px] font-medium text-ink">{loop.loop_name || 'Loop'}</p>
      {meta ? <p className="mt-1 text-[12.5px] tabular-nums text-ink-3">{meta}</p> : null}
      <div className="mt-3 flex items-center justify-between gap-3 border-t border-border/70 pt-3">
        {loop.boards?.length ? (
          <span className="flex flex-wrap items-center gap-x-1.5 gap-y-1 text-[12.5px] text-ink-3">
            <span>On board</span>
            {loop.boards.map((board, index) => (
              <span key={board.id} className="inline-flex items-center">
                {index > 0 ? <span className="mr-1.5 text-ink-3">·</span> : null}
                <Link
                  to="/boards/$boardId"
                  params={{ boardId: board.id }}
                  className="text-ink-2 transition-colors hover:text-primary"
                >
                  {board.name || 'board'}
                </Link>
              </span>
            ))}
          </span>
        ) : (
          <span />
        )}
        <Link
          to="/loops/$loopId"
          params={{ loopId: loop.loop_id }}
          className="shrink-0 text-[12.5px] text-ink-2 transition-colors hover:text-primary"
        >
          View loop →
        </Link>
      </div>
    </div>
  )
}
