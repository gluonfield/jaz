import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useToast } from '@/components/ui/toast'
import { boardsQuery } from '@/lib/api/boards'
import { type LoopDetail, updateLoop } from '@/lib/api/loops'
import type { Loop } from '@/lib/api/types'
import { keys } from '@/lib/query/keys'
import { BoardAssignmentPicker } from './BoardAssignmentPicker'

export function LoopBoardAssignments({ loop, boardIds }: { loop: Loop; boardIds: string[] }) {
  const boards = useQuery(boardsQuery)
  const queryClient = useQueryClient()
  const toast = useToast()

  const assign = useMutation<Loop, Error, string[]>({
    mutationFn: (nextBoardIds) => updateLoop(loop.id, { board_ids: nextBoardIds }),
    onSuccess: (_saved, nextBoardIds) => {
      queryClient.setQueryData<LoopDetail>(keys.loopDetail(loop.id), (current) =>
        current ? { ...current, boardIds: nextBoardIds } : current,
      )
      queryClient.invalidateQueries({ queryKey: keys.loopDetail(loop.id) })
      queryClient.invalidateQueries({ queryKey: keys.loops })
      queryClient.invalidateQueries({ queryKey: keys.boards })
    },
    onError: (error) => toast(`Couldn't update boards: ${error.message}`, 'danger'),
  })

  const allBoards = boards.data ?? []
  const selected = assign.isPending && assign.variables ? assign.variables : boardIds
  const availableBoardIds = new Set(allBoards.map((board) => board.id))
  const activeCount = selected.filter((id) => availableBoardIds.has(id)).length

  return (
    <section className="mt-8">
      <div className="flex items-baseline justify-between border-b border-border pb-2">
        <h2 className="text-[13px] font-semibold text-ink">Widget boards</h2>
        {allBoards.length > 0 ? (
          <span className="text-[12px] tabular-nums text-ink-3">
            {activeCount} of {allBoards.length} active
          </span>
        ) : null}
      </div>
      <div className="pt-3">
        {boards.isPending ? (
          <p className="text-[13px] text-ink-3">Loading boards…</p>
        ) : boards.isError ? (
          <p role="alert" className="text-[13px] text-danger">
            Couldn’t load boards: {boards.error.message}
          </p>
        ) : allBoards.length === 0 ? (
          <p className="text-[13px] text-ink-3">No boards yet. Create a board to show this widget.</p>
        ) : (
          <BoardAssignmentPicker
            boards={allBoards}
            selected={selected}
            disabled={assign.isPending}
            onChange={(next) => assign.mutate(next)}
            hint="Checked boards show this loop's widget and receive updates on every run."
          />
        )}
        {assign.isError ? (
          <p role="alert" className="mt-2 text-[12px] text-danger">
            {assign.error.message}
          </p>
        ) : null}
      </div>
    </section>
  )
}
