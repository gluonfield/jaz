import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { Check, Plus } from 'lucide-react'
import { motion, useReducedMotion } from 'motion/react'
import { type ReactNode, useState } from 'react'
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
      size="lg"
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
          <BoardIllustration />
          <p className="text-[13px] leading-relaxed text-ink-2">
            Assign a loop to a board and every run rewrites its widget — counts, lists, charts —
            using data it just gathered. Drag and resize tiles; your layout always wins.
          </p>
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

// A miniature mission control so the explanation shows a board, not a lone
// widget: live-looking tiles on the dotted canvas plus a free cell, with the
// rainbow scanline sweeping fresh data into the first tile.
function BoardIllustration() {
  const reduce = useReducedMotion()
  const tileIn = (delay: number) =>
    reduce
      ? {}
      : {
          initial: { opacity: 0, y: 10, scale: 0.97 },
          animate: { opacity: 1, y: 0, scale: 1 },
          transition: { delay, duration: 0.35, ease: [0.22, 1, 0.36, 1] as const },
        }
  return (
    <div
      aria-hidden
      className="rounded-card bg-bg p-3 ring-1 ring-border"
      style={{
        backgroundImage: 'radial-gradient(var(--color-border) 1px, transparent 1px)',
        backgroundSize: '16px 16px',
      }}
    >
      <div className="grid grid-cols-3 gap-2.5">
        <motion.div {...tileIn(0.05)} className="col-span-2 h-[108px]">
          <PullRequestsTile />
        </motion.div>
        <motion.div {...tileIn(0.14)} className="h-[108px]">
          <DeploysTile />
        </motion.div>
        <motion.div {...tileIn(0.23)} className="h-[96px]">
          <TokensTile />
        </motion.div>
        <motion.div {...tileIn(0.3)} className="h-[96px]">
          <InboxTile />
        </motion.div>
        <motion.div {...tileIn(0.4)} className="h-[96px]">
          <div className="flex h-full items-center justify-center gap-1.5 rounded-card border border-dashed border-border text-[11px] text-ink-3">
            <Plus size={12} />
            New widget
          </div>
        </motion.div>
      </div>
    </div>
  )
}

function TileFrame({
  title,
  time,
  dot,
  children,
}: {
  title: string
  time: string
  dot?: ReactNode
  children: ReactNode
}) {
  return (
    <div className="flex h-full flex-col overflow-hidden rounded-card bg-surface ring-1 ring-border">
      <div className="flex h-7 shrink-0 items-center gap-1.5 px-2.5">
        {dot ?? <span className="size-1.5 shrink-0 rounded-full bg-primary" />}
        <span className="min-w-0 flex-1 truncate text-[11px] font-medium text-ink">{title}</span>
        <span className="shrink-0 text-[10px] tabular-nums text-ink-3">{time}</span>
      </div>
      <div className="min-h-0 flex-1 px-2.5 pb-2.5">{children}</div>
    </div>
  )
}

// The fresh-data moment from the real board: a vertical rainbow band sweeping
// the tile left to right, comet-masked, then gone — rainbow only in motion.
function Scanline() {
  return (
    <motion.div
      className="pointer-events-none absolute inset-0 z-10"
      style={{
        background:
          'linear-gradient(180deg, var(--color-rainbow-1), var(--color-rainbow-2), var(--color-rainbow-3), var(--color-rainbow-4), var(--color-rainbow-5))',
        maskImage: 'linear-gradient(90deg, transparent 30%, black 50%, transparent 54%)',
        WebkitMaskImage: 'linear-gradient(90deg, transparent 30%, black 50%, transparent 54%)',
      }}
      initial={{ x: '-60%', opacity: 0 }}
      animate={{ x: ['-60%', '-24%', '24%', '60%'], opacity: [0, 0.85, 0.85, 0] }}
      transition={{
        delay: 1.2,
        duration: 0.7,
        ease: 'easeInOut',
        times: [0, 0.12, 0.82, 1],
        repeat: Infinity,
        repeatDelay: 4.5,
      }}
    />
  )
}

function PullRequestsTile() {
  const reduce = useReducedMotion()
  const bars = [35, 60, 45, 85, 65, 52, 74]
  return (
    <div className="relative h-full overflow-hidden rounded-card">
      <TileFrame title="Open PRs" time="2m">
        <div className="flex h-full items-end gap-5 pb-0.5">
          <div className="shrink-0">
            <div className="text-[20px] font-semibold leading-tight tabular-nums text-ink">7</div>
            <div className="text-[10px] text-ink-2">open</div>
            <div className="text-[10px] font-medium text-ok">+2 today</div>
          </div>
          <div className="flex h-12 flex-1 items-end gap-1">
            {bars.map((height, index) => (
              <motion.div
                key={index}
                initial={reduce ? false : { height: 0 }}
                animate={{ height: `${height}%` }}
                transition={{ delay: 0.4 + index * 0.06, duration: 0.4, ease: [0.22, 1, 0.36, 1] }}
                className={`flex-1 rounded-t-[3px] ${index === 4 ? 'bg-accent' : 'bg-primary/80'}`}
              />
            ))}
          </div>
        </div>
      </TileFrame>
      {reduce ? null : <Scanline />}
    </div>
  )
}

function DeploysTile() {
  const services = [
    { name: 'api', detail: 'v214', dot: 'bg-ok' },
    { name: 'web', detail: 'v213', dot: 'bg-ok' },
    { name: 'worker', detail: 'deploying', dot: 'bg-running animate-pulse' },
  ]
  return (
    <TileFrame
      title="Deploys"
      time="now"
      dot={<span className="size-1.5 shrink-0 animate-pulse rounded-full bg-running" />}
    >
      <div className="flex h-full flex-col justify-center gap-1.5">
        {services.map((service) => (
          <div key={service.name} className="flex items-center gap-1.5">
            <span className={`size-1.5 shrink-0 rounded-full ${service.dot}`} />
            <span className="min-w-0 flex-1 truncate text-[11px] text-ink-2">{service.name}</span>
            <span className="shrink-0 text-[10px] tabular-nums text-ink-3">{service.detail}</span>
          </div>
        ))}
      </div>
    </TileFrame>
  )
}

function TokensTile() {
  const reduce = useReducedMotion()
  return (
    <TileFrame title="Tokens" time="5m">
      <div className="flex h-full flex-col justify-end gap-1 pb-0.5">
        <div className="flex items-baseline gap-1.5">
          <span className="text-[15px] font-semibold tabular-nums text-ink">312k</span>
          <span className="text-[10px] font-medium text-ok">−18%</span>
        </div>
        <svg viewBox="0 0 100 28" className="h-7 w-full" preserveAspectRatio="none">
          <motion.path
            d="M0 22 C 10 20, 16 12, 26 14 S 44 25, 54 18 S 70 4, 80 8 S 94 13, 100 9"
            fill="none"
            stroke="var(--color-primary)"
            strokeWidth="2"
            strokeLinecap="round"
            vectorEffect="non-scaling-stroke"
            initial={reduce ? false : { pathLength: 0 }}
            animate={{ pathLength: 1 }}
            transition={{ delay: 0.5, duration: 0.9, ease: 'easeOut' }}
          />
        </svg>
      </div>
    </TileFrame>
  )
}

function InboxTile() {
  const rows = [
    { text: '2 PRs need review', dot: 'bg-accent' },
    { text: 'CI green on main', dot: 'bg-ok' },
    { text: '1 flaky test', dot: 'bg-danger' },
  ]
  return (
    <TileFrame title="Inbox triage" time="12m">
      <div className="flex h-full flex-col justify-center gap-1.5">
        {rows.map((row) => (
          <div key={row.text} className="flex items-center gap-1.5">
            <span className={`size-1.5 shrink-0 rounded-full ${row.dot}`} />
            <span className="min-w-0 flex-1 truncate text-[11px] text-ink-2">{row.text}</span>
          </div>
        ))}
      </div>
    </TileFrame>
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
