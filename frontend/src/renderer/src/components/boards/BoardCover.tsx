import { useQuery } from '@tanstack/react-query'
import { boardDetailQuery } from '@/lib/api/boards'
import { loopTone, TONE_DOT } from '@/lib/api/loops'
import type { Board, BoardItem } from '@/lib/api/types'

// A deliberately simple preview: the board's first widgets in reading order on
// a uniform grid. Real layouts can overlap, so the cover never mirrors x/y/w/h.
const COVER_TILES = 6

export function BoardCover({ board }: { board: Board }) {
  // The index only needs a snapshot; the detail page owns live polling.
  const detail = useQuery({ ...boardDetailQuery(board.id), refetchInterval: false })
  const tiles = [...(detail.data?.items ?? [])]
    .sort((a, b) => a.y - b.y || a.x - b.x)
    .slice(0, COVER_TILES)
  return (
    <div
      aria-hidden
      className="overflow-hidden rounded-card bg-bg p-2.5 ring-1 ring-border/60 transition-shadow duration-200 group-hover:shadow-[0_10px_28px_-14px_rgb(0_0_0/0.3)] group-hover:ring-primary/35"
      style={{
        backgroundImage: 'radial-gradient(var(--color-border) 1px, transparent 1px)',
        backgroundSize: '14px 14px',
      }}
    >
      <div className="grid aspect-[16/9.2] grid-cols-3 grid-rows-2 gap-1.5">
        {detail.isPending ? (
          [0, 1, 2].map((i) => <div key={i} className="animate-pulse rounded-[5px] bg-surface" />)
        ) : tiles.length > 0 ? (
          tiles.map((item) => <CoverTile key={item.widget_id} item={item} />)
        ) : (
          <span className="col-span-full row-span-full place-self-center rounded-[5px] border border-dashed border-border px-2 py-1 text-[10px] text-ink-3">
            {detail.isError ? 'Preview unavailable' : 'Empty board'}
          </span>
        )}
      </div>
    </div>
  )
}

function CoverTile({ item }: { item: BoardItem }) {
  const dot = TONE_DOT[item.last_error ? 'failed' : loopTone(item.loop_last_run_status, item.loop_status)]
  return (
    <div className="flex min-h-0 flex-col overflow-hidden rounded-[5px] bg-surface p-1.5">
      <div className="flex shrink-0 items-center gap-1">
        <span className={`size-1 shrink-0 rounded-full ${dot}`} />
        <span className="min-w-0 truncate text-[9px] font-medium leading-tight text-ink-2">
          {item.title || item.loop_name}
        </span>
      </div>
      <div className="mt-1.5 flex min-h-0 flex-col gap-1 overflow-hidden">
        <span className="h-1 w-4/5 rounded-full bg-ink/10" />
        <span className="h-1 w-3/5 rounded-full bg-ink/10" />
      </div>
    </div>
  )
}
