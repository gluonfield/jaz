import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { Check, LayoutGrid } from 'lucide-react'
import { motion, useReducedMotion } from 'motion/react'
import { useState } from 'react'
import { compactSchedule } from '@/components/loops/schedule'
import { Button } from '@/components/ui/Button'
import { Modal } from '@/components/ui/Modal'
import { assignLoopsToBoard, createBoard } from '@/lib/api/boards'
import { loopsQuery } from '@/lib/api/loops'
import type { Board, Loop } from '@/lib/api/types'
import { keys } from '@/lib/query/keys'

// Two-step board onboarding in a modal: explain + create, then assign loops —
// assignment is what turns a loop's widget on.
export function BoardModal({ open, onClose }: { open: boolean; onClose: () => void }) {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const loops = useQuery(loopsQuery)
  const [board, setBoard] = useState<Board | null>(null)
  const [name, setName] = useState('')
  const [selected, setSelected] = useState<string[]>([])

  const close = () => {
    setBoard(null)
    setName('')
    setSelected([])
    onClose()
  }

  // Board rows open inline; the board page has the dedicated pop-out control.
  const openBoard = (target: Board) => {
    close()
    void navigate({ to: '/boards/$boardId', params: { boardId: target.id } })
  }

  const create = useMutation({
    mutationFn: () => createBoard(name.trim() || 'Board'),
    onSuccess: (created) => {
      queryClient.invalidateQueries({ queryKey: keys.boards })
      setBoard(created)
    },
  })

  const assign = useMutation({
    mutationFn: () => assignLoopsToBoard(board!.id, selected),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: keys.boards })
      if (board) queryClient.invalidateQueries({ queryKey: keys.boardDetail(board.id) })
      if (board) openBoard(board)
    },
  })

  const toggle = (id: string) =>
    setSelected((current) =>
      current.includes(id) ? current.filter((s) => s !== id) : [...current, id],
    )
  const available = (loops.data ?? []).filter((loop) => loop.status !== 'deleted')
  const error = create.isError
    ? (create.error as Error).message
    : assign.isError
      ? (assign.error as Error).message
      : ''

  return (
    <Modal
      open={open}
      onClose={close}
      size="md"
      icon={<LayoutGrid size={16} />}
      title={board ? `Add loops to ${board.name}` : 'New board'}
      description={
        board
          ? 'Assigning a loop is what turns its widget on. Change it anytime from the loop.'
          : 'A tiled board your loops keep up to date.'
      }
      footer={
        <>
          <p className="text-[12px] text-danger" role="alert">
            {error}
          </p>
          <div className="flex shrink-0 items-center gap-1">
            {board ? (
              <>
                <Button variant="ghost" size="md" onClick={() => openBoard(board)}>
                  Skip for now
                </Button>
                <Button
                  variant="primary"
                  size="md"
                  disabled={selected.length === 0 || assign.isPending}
                  onClick={() => assign.mutate()}
                >
                  {assign.isPending
                    ? 'Adding…'
                    : selected.length > 0
                      ? `Add ${selected.length} ${selected.length === 1 ? 'loop' : 'loops'}`
                      : 'Add loops'}
                </Button>
              </>
            ) : (
              <>
                <Button variant="ghost" size="md" onClick={close}>
                  Cancel
                </Button>
                <Button
                  variant="primary"
                  size="md"
                  disabled={create.isPending}
                  onClick={() => create.mutate()}
                >
                  {create.isPending ? 'Creating…' : 'Create board'}
                </Button>
              </>
            )}
          </div>
        </>
      }
    >
      {board ? (
        loops.isPending ? (
          <p className="text-[13px] text-ink-3">Loading loops…</p>
        ) : available.length === 0 ? (
          <p className="rounded-card bg-surface px-4 py-3 text-[13px] text-ink-2">
            No loops yet. Create one from the sidebar — you can add it to this board from the loop
            form.
          </p>
        ) : (
          <div className="flex flex-col gap-1">
            {available.map((loop) => (
              <LoopPickRow
                key={loop.id}
                loop={loop}
                selected={selected.includes(loop.id)}
                onToggle={() => toggle(loop.id)}
              />
            ))}
          </div>
        )
      ) : (
        <div className="space-y-5">
          <p className="text-[13px] leading-relaxed text-ink-2">
            Assign a loop to a board and every run rewrites its widget — counts, lists, charts —
            using data it just gathered. Drag and resize tiles; your layout always wins.
          </p>
          <DemoTile />
          <label className="block">
            <span className="mb-1.5 block text-[12px] font-medium text-ink-2">Name</span>
            <input
              type="text"
              value={name}
              autoFocus
              onChange={(e) => setName(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter' && !create.isPending) create.mutate()
              }}
              placeholder="Mission control"
              className="w-full rounded-control bg-bg px-3 py-2 text-[13px] text-ink ring-1 ring-border outline-none transition duration-150 placeholder:text-ink-3 focus:ring-primary"
            />
          </label>
        </div>
      )}
    </Modal>
  )
}

// A static mock of a widget tile so the explanation has something to point at.
function DemoTile() {
  const reduce = useReducedMotion()
  const bars = [35, 60, 45, 85, 65]
  return (
    <div className="overflow-hidden rounded-card bg-surface ring-1 ring-border">
      <div className="flex h-8 items-center gap-1.5 px-2.5">
        <span className="size-1.5 rounded-full bg-primary" />
        <span className="flex-1 text-[12px] font-medium text-ink">Open PRs</span>
        <span className="text-[11px] tabular-nums text-ink-3">2m</span>
      </div>
      <div className="flex items-end gap-6 px-4 pb-4 pt-1">
        <div>
          <div className="text-[22px] font-semibold leading-tight tabular-nums text-ink">7</div>
          <div className="text-[11px] text-ink-2">open</div>
          <div className="text-[11px] font-medium text-ok">+2 today</div>
        </div>
        <div className="flex h-14 flex-1 items-end gap-1.5">
          {bars.map((height, index) => (
            <motion.div
              key={index}
              initial={reduce ? false : { height: 0 }}
              animate={{ height: `${height}%` }}
              transition={{ delay: 0.25 + index * 0.07, duration: 0.4, ease: [0.22, 1, 0.36, 1] }}
              className={`flex-1 rounded-t-[3px] ${index === 3 ? 'bg-accent' : 'bg-primary/80'}`}
            />
          ))}
        </div>
      </div>
    </div>
  )
}

function LoopPickRow({
  loop,
  selected,
  onToggle,
}: {
  loop: Loop
  selected: boolean
  onToggle: () => void
}) {
  return (
    <button
      type="button"
      onClick={onToggle}
      aria-pressed={selected}
      className={`flex items-center gap-2.5 rounded-control px-3 py-2 text-left ring-1 transition-colors duration-150 ${
        selected ? 'bg-primary-soft ring-primary/40' : 'bg-bg ring-border hover:bg-surface'
      }`}
    >
      <span
        className={`grid size-4 shrink-0 place-items-center rounded-full ring-1 transition-colors duration-150 ${
          selected ? 'bg-primary text-on-primary ring-primary' : 'bg-bg text-transparent ring-border'
        }`}
      >
        <Check size={10} />
      </span>
      <span className="min-w-0 flex-1 truncate text-[13px] font-medium text-ink">{loop.name}</span>
      <span className="shrink-0 text-[11px] tabular-nums text-ink-3">
        {compactSchedule(loop.schedule?.expr ?? '', loop.status === 'paused')}
      </span>
    </button>
  )
}
