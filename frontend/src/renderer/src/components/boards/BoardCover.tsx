import { useQuery } from '@tanstack/react-query'
import { motion, useReducedMotion } from 'motion/react'
import { SCANLINE_BACKGROUND, SCANLINE_MASK } from '@/components/ui/rainbow'
import { boardDetailQuery } from '@/lib/api/boards'
import { activeRunStatus } from '@/lib/api/loops'
import type { Board, BoardItem } from '@/lib/api/types'

// Rows shown before the miniature crops; tall boards fade below the fold.
const COVER_ROW_CAP = 4

// A real miniature of the board: its actual widget layout at postage-stamp
// scale, so every card previews exactly the board it opens.
export function BoardCover({ board }: { board: Board }) {
  const reduce = useReducedMotion()
  // The index only needs a snapshot; the detail page owns live polling.
  const detail = useQuery({ ...boardDetailQuery(board.id), refetchInterval: false })
  const items = detail.data?.items ?? []
  const cols = board.grid_cols > 0 ? board.grid_cols : 6
  const rows = Math.min(COVER_ROW_CAP, Math.max(2, ...items.map((item) => item.y + item.h)))
  return (
    <motion.div
      aria-hidden
      whileHover={reduce ? undefined : 'hover'}
      className="relative overflow-hidden rounded-card bg-bg p-2.5 ring-1 ring-border/60 transition-[box-shadow,--tw-ring-color] duration-200 group-hover:shadow-[0_10px_28px_-14px_rgb(0_0_0/0.3)] group-hover:ring-primary/35"
      style={{
        backgroundImage: 'radial-gradient(var(--color-border) 1px, transparent 1px)',
        backgroundSize: '14px 14px',
      }}
    >
      <div className="aspect-[16/9.2] overflow-hidden">
        {items.length > 0 ? (
          <div
            className="grid h-full gap-1.5"
            style={{
              gridTemplateColumns: `repeat(${cols}, minmax(0, 1fr))`,
              gridTemplateRows: `repeat(${rows}, minmax(0, 1fr))`,
              gridAutoRows: `${100 / rows}%`,
            }}
          >
            {items.map((item) => (
              <CoverTile key={item.widget_id} item={item} />
            ))}
          </div>
        ) : detail.isPending ? (
          <div className="grid h-full grid-cols-3 grid-rows-2 gap-1.5">
            <div className="col-span-2 animate-pulse rounded-[5px] bg-surface" />
            <div className="animate-pulse rounded-[5px] bg-surface" />
            <div className="animate-pulse rounded-[5px] bg-surface" />
          </div>
        ) : (
          <div className="grid h-full place-items-center">
            <span className="rounded-[5px] border border-dashed border-border px-2 py-1 text-[10px] text-ink-3">
              Empty board
            </span>
          </div>
        )}
      </div>
      <motion.div
        className="pointer-events-none absolute inset-0"
        style={{ background: SCANLINE_BACKGROUND, maskImage: SCANLINE_MASK, WebkitMaskImage: SCANLINE_MASK, x: '-60%', opacity: 0 }}
        variants={{
          hover: { x: ['-60%', '60%'], opacity: [0, 0.85, 0.85, 0], transition: { duration: 0.6, ease: 'easeInOut' } },
        }}
      />
    </motion.div>
  )
}

function CoverTile({ item }: { item: BoardItem }) {
  const dot =
    item.loop_last_run_status === 'error' || item.last_error
      ? 'bg-danger'
      : activeRunStatus(item.loop_last_run_status)
        ? 'bg-running animate-pulse'
        : item.loop_status === 'paused'
          ? 'bg-ink-3/40'
          : 'bg-primary'
  return (
    <div
      style={{ gridColumn: `${item.x + 1} / span ${item.w}`, gridRow: `${item.y + 1} / span ${item.h}` }}
      className="flex min-h-0 flex-col overflow-hidden rounded-[5px] bg-surface p-1.5"
    >
      <div className="flex shrink-0 items-center gap-1">
        <span className={`size-1 shrink-0 rounded-full ${dot}`} />
        <span className="min-w-0 truncate text-[9px] font-medium leading-tight text-ink-2">
          {item.title || item.loop_name}
        </span>
      </div>
      <div className="mt-1.5 flex min-h-0 flex-col gap-1 overflow-hidden">
        <span className="h-1 w-4/5 rounded-full bg-ink/10" />
        {item.h > 1 ? <span className="h-1 w-3/5 rounded-full bg-ink/10" /> : null}
      </div>
    </div>
  )
}
