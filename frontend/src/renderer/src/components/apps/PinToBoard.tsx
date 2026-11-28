import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { LayoutDashboard } from 'lucide-react'
import { useState } from 'react'
import { Button } from '@/components/ui/Button'
import { useToast } from '@/components/ui/toast'
import { addBoardTiles, boardsQuery } from '@/lib/api/boards'
import { keys } from '@/lib/query/keys'

// Puts a published app on one of the boards as a tile.
export function PinToBoard({ appId }: { appId: string }) {
  const queryClient = useQueryClient()
  const toast = useToast()
  const boards = useQuery(boardsQuery)
  const [boardId, setBoardId] = useState('')

  const pin = useMutation({
    mutationFn: () => addBoardTiles(boardId, { app_ids: [appId] }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: keys.appBoard(boardId) })
      toast('Pinned app to board')
      setBoardId('')
    },
    onError: (err) => toast(err.message, 'danger'),
  })

  return (
    <div className="rounded-control border border-border bg-surface p-3">
      <div className="mb-1.5 flex items-center gap-1.5 text-[12px] font-medium text-ink">
        <LayoutDashboard size={13} />
        Pin to board
      </div>
      <div className="flex items-center gap-2">
        <select
          value={boardId}
          onChange={(e) => setBoardId(e.target.value)}
          className="min-w-0 flex-1 rounded-control border border-border bg-bg px-2.5 py-1.5 text-[12.5px] text-ink"
        >
          <option value="">Choose a board…</option>
          {(boards.data ?? []).map((board) => (
            <option key={board.id} value={board.id}>{board.title}</option>
          ))}
        </select>
        <Button variant="secondary" size="sm" disabled={!boardId || pin.isPending} onClick={() => pin.mutate()}>
          {pin.isPending ? 'Pinning…' : 'Pin'}
        </Button>
      </div>
    </div>
  )
}
