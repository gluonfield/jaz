import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { useEffect } from 'react'
import { EmptyState } from '@/components/ui/EmptyState'
import { boardsQuery } from '@/lib/api/boards'

export const Route = createFileRoute('/boards/')({
  component: BoardsIndex,
})

// Listing the boards lives in the sidebar; /boards lands on the first board.
function BoardsIndex() {
  const boards = useQuery(boardsQuery)
  const navigate = useNavigate()

  useEffect(() => {
    const board = boards.data?.[0]
    if (board) {
      void navigate({ to: '/boards/$boardId', params: { boardId: board.id }, replace: true })
    }
  }, [boards.data, navigate])

  if (boards.isError) {
    return <EmptyState title="Boards unavailable">Could not load boards.</EmptyState>
  }
  if (boards.data && boards.data.length === 0) {
    return (
      <EmptyState title="No boards yet">
        Create one with the + next to Boards in the sidebar.
      </EmptyState>
    )
  }
  return null
}
