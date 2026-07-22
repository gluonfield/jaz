import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, createFileRoute } from '@tanstack/react-router'
import { ExternalLink, Plus, Trash2 } from 'lucide-react'
import { motion, useReducedMotion } from 'motion/react'
import { type ReactNode, useState } from 'react'
import { BoardCover } from '@/components/boards/BoardCover'
import { BoardModal } from '@/components/boards/BoardModal'
import { Button } from '@/components/ui/Button'
import { DashedCta } from '@/components/ui/DashedCta'
import { EmptyState } from '@/components/ui/EmptyState'
import { IconButton } from '@/components/ui/IconButton'
import { SkeletonRows } from '@/components/ui/Skeleton'
import { boardsQuery, deleteBoard } from '@/lib/api/boards'
import type { Board } from '@/lib/api/types'
import { popOutBoard } from '@/lib/clientRuntime'
import { keys } from '@/lib/query/keys'

export const Route = createFileRoute('/boards/')({
  component: BoardsPage,
})

function BoardsPage() {
  const boards = useQuery(boardsQuery)
  const queryClient = useQueryClient()
  const [creating, setCreating] = useState(false)

  const remove = useMutation({
    mutationFn: (boardId: string) => deleteBoard(boardId),
    onSettled: () => queryClient.invalidateQueries({ queryKey: keys.boards }),
  })

  const onDelete = (board: Board) => {
    if (
      confirm(
        `Delete board "${board.name}"? Widgets and their history are kept — loops just stop publishing until you assign them to another board.`,
      )
    ) {
      remove.mutate(board.id)
    }
  }

  return (
    <div className="mx-auto max-w-[960px] px-10 pb-16 pt-6">
      <header className="flex items-end justify-between pb-6">
        <div>
          <h1 className="text-[22px] font-semibold tracking-[-0.01em] text-ink">Boards</h1>
          <p className="mt-1 text-[13px] text-ink-3">Tiled dashboards your loops keep up to date.</p>
        </div>
        <Button variant="primary" size="lg" onClick={() => setCreating(true)}>
          <Plus size={15} />
          New board
        </Button>
      </header>

      {boards.isPending ? (
        <SkeletonRows count={4} />
      ) : boards.isError ? (
        <EmptyState title="Boards unavailable">Could not load boards.</EmptyState>
      ) : boards.data.length === 0 ? (
        <DashedCta
          onClick={() => setCreating(true)}
          title="Create your first board"
          subtitle="Assign loops to a board and every run rewrites its widgets — counts, lists, charts — from data they just gathered."
        />
      ) : (
        <div className="grid gap-5 sm:grid-cols-2 lg:grid-cols-3">
          {boards.data.map((board, index) => (
            <BoardCard key={board.id} board={board} index={index} onDelete={() => onDelete(board)} />
          ))}
          <NewBoardTile index={boards.data.length} onClick={() => setCreating(true)} />
        </div>
      )}

      <BoardModal open={creating} onClose={() => setCreating(false)} />
    </div>
  )
}

// Staggered entrance shared by cards and the create tile; the delay cap keeps
// late grid cells from sitting invisible on board-heavy accounts.
function CardIn({ index, className, children }: { index: number; className?: string; children: ReactNode }) {
  const reduce = useReducedMotion()
  return (
    <motion.div
      initial={reduce ? false : { opacity: 0, y: 10 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.35, ease: [0.22, 1, 0.36, 1], delay: Math.min(index, 8) * 0.05 }}
      className={className}
    >
      {children}
    </motion.div>
  )
}

function BoardCard({ board, index, onDelete }: { board: Board; index: number; onDelete: () => void }) {
  return (
    <CardIn index={index} className="group relative">
      <Link
        to="/boards/$boardId"
        params={{ boardId: board.id }}
        className="block rounded-card outline-none focus-visible:ring-2 focus-visible:ring-primary/40"
      >
        <BoardCover board={board} />
        <p className="truncate px-1 pt-2 text-[14px] font-medium text-ink" title={board.name}>
          {board.name}
        </p>
      </Link>
      <span className="absolute right-2 top-2 flex gap-1 opacity-0 transition-opacity duration-150 focus-within:opacity-100 group-hover:opacity-100 max-sm:opacity-100">
        <IconButton
          variant="ghost"
          size="sm"
          aria-label={`Pop out board ${board.name}`}
          title="Pop out board"
          onClick={() => popOutBoard(board.id)}
          className="bg-bg/80 shadow-sm backdrop-blur-sm"
        >
          <ExternalLink size={13} />
        </IconButton>
        <IconButton
          variant="ghost"
          size="sm"
          aria-label={`Delete board ${board.name}`}
          title="Delete board"
          onClick={onDelete}
          className="bg-bg/80 shadow-sm backdrop-blur-sm"
        >
          <Trash2 size={13} />
        </IconButton>
      </span>
    </CardIn>
  )
}

function NewBoardTile({ index, onClick }: { index: number; onClick: () => void }) {
  return (
    <CardIn index={index}>
      <motion.button
        type="button"
        onClick={onClick}
        whileTap={{ scale: 0.98 }}
        className="flex aspect-[16/10] w-full flex-col items-center justify-center gap-2 rounded-card border border-dashed border-border text-ink-3 transition-colors duration-150 hover:border-primary/50 hover:bg-surface hover:text-ink"
      >
        <Plus size={16} />
        <span className="text-[13px] font-medium">New board</span>
      </motion.button>
    </CardIn>
  )
}
