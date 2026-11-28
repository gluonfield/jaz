import { useQuery } from '@tanstack/react-query'
import { ThumbnailSheet } from '@/components/apps/AppThumbnail'
import { boardViewQuery, type BoardTile } from '@/lib/api/boards'
import { loopTone, TONE_DOT } from '@/lib/api/loops'

// A row's height as a share of a column's width, as on a board in a wide window.
const ROW_RATIO = 0.74

// A board in the library: its tiles where the board has them, on the sheet an
// app's preview sits on. Rows past the sheet's foot run off it.
export function BoardThumbnail({ boardId }: { boardId: string }) {
  // The library shows a snapshot; the board's own page polls.
  const view = useQuery({ ...boardViewQuery(boardId), refetchInterval: false })
  const tiles = view.data?.tiles ?? []
  const column = 100 / (view.data?.grid_cols ?? 1)
  return (
    <ThumbnailSheet>
      <div aria-hidden className="size-full px-[4.5%] pt-[4.5%]">
        {tiles.length > 0 ? (
          <div className="@container relative size-full">
            {tiles.map((tile) => (
              <div
                key={tile.app.id}
                className="absolute p-[1.5px]"
                style={{
                  left: `${tile.x * column}cqw`,
                  top: `${tile.y * column * ROW_RATIO}cqw`,
                  width: `${tile.w * column}cqw`,
                  height: `${tile.h * column * ROW_RATIO}cqw`,
                }}
              >
                <MiniTile tile={tile} />
              </div>
            ))}
          </div>
        ) : view.isPending ? (
          <div className="grid grid-cols-3 gap-[3px]">
            {[0, 1, 2].map((index) => <div key={index} className="aspect-[4/3] animate-pulse rounded-[4px] bg-surface" />)}
          </div>
        ) : (
          <div className="grid size-full place-items-center pb-[6%]">
            <span className="rounded-[5px] border border-dashed border-border px-2 py-1 text-[10.5px] text-ink-3">
              {view.isError ? 'Preview unavailable' : 'No tiles yet'}
            </span>
          </div>
        )}
      </div>
    </ThumbnailSheet>
  )
}

function MiniTile({ tile }: { tile: BoardTile }) {
  const dot = tile.loop
    ? TONE_DOT[tile.last_error ? 'failed' : loopTone(tile.loop.last_run_status, tile.loop.status)]
    : 'bg-ink/25'
  return (
    <div className="flex size-full flex-col gap-[3px] overflow-hidden rounded-[4px] bg-surface p-[4px] ring-1 ring-ink/[0.08]">
      <div className="flex shrink-0 items-center gap-[3px]">
        <span className={`size-[3px] shrink-0 rounded-full ${dot}`} />
        <span className="min-w-0 truncate text-[7px] font-medium leading-[1.2] text-ink-2">{tile.app.title}</span>
      </div>
      <span className="h-[3px] w-4/5 shrink-0 rounded-full bg-ink/10" />
      <span className="h-[3px] w-1/2 shrink-0 rounded-full bg-ink/10" />
      {tile.h > 1 ? <span className="mt-px min-h-0 flex-1 rounded-[3px] bg-ink/[0.05]" /> : null}
    </div>
  )
}
