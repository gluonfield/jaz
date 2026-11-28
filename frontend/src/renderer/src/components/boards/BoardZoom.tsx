import { useMutation, useQueryClient } from '@tanstack/react-query'
import { ZoomIn, ZoomOut } from 'lucide-react'
import { useEffect, useRef } from 'react'
import { IconButton } from '@/components/ui/IconButton'
import { patchBoard, type BoardView } from '@/lib/api/boards'
import { keys } from '@/lib/query/keys'

const MIN_SCALE = 0.7
const MAX_SCALE = 2

// Zooms the text of every tile. Anyone can zoom their own view; an editor's
// zoom is saved with the board.
export function BoardZoom({ boardId, scale, save }: { boardId: string; scale: number; save: boolean }) {
  const queryClient = useQueryClient()
  const timer = useRef<number | null>(null)
  const viewKey = keys.appBoard(boardId)

  const saveScale = useMutation({
    mutationFn: (fontScale: number) => patchBoard(boardId, { font_scale: fontScale }),
    onSettled: () => queryClient.invalidateQueries({ queryKey: viewKey }),
  })

  useEffect(() => () => {
    if (timer.current) window.clearTimeout(timer.current)
  }, [])

  // A step shows at once; the save waits so quick steps settle on one value.
  const apply = (next: number) => {
    queryClient.setQueryData<BoardView>(viewKey, (prev) => prev && { ...prev, font_scale: next })
    if (!save) return
    if (timer.current) window.clearTimeout(timer.current)
    timer.current = window.setTimeout(() => saveScale.mutate(next), 400)
  }
  const step = (delta: number) =>
    apply(Math.min(MAX_SCALE, Math.max(MIN_SCALE, Math.round((scale + delta) * 10) / 10)))

  return (
    <div className="flex items-center">
      <IconButton size="sm" aria-label="Smaller tile text" title="Smaller tile text" disabled={scale <= MIN_SCALE} onClick={() => step(-0.1)}>
        <ZoomOut size={14} />
      </IconButton>
      {scale !== 1 ? (
        <button
          type="button"
          title="Reset tile text size"
          onClick={() => apply(1)}
          className="cursor-pointer rounded-full px-1 text-[11.5px] tabular-nums text-ink-3 transition-colors duration-150 hover:text-ink"
        >
          {Math.round(scale * 100)}%
        </button>
      ) : null}
      <IconButton size="sm" aria-label="Larger tile text" title="Larger tile text" disabled={scale >= MAX_SCALE} onClick={() => step(0.1)}>
        <ZoomIn size={14} />
      </IconButton>
    </div>
  )
}
