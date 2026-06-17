import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Check, Plus } from 'lucide-react'
import { motion } from 'motion/react'
import { useState } from 'react'
import { compactSchedule } from '@/components/loops/schedule'
import { Button } from '@/components/ui/Button'
import { DashedCta } from '@/components/ui/DashedCta'
import { Input } from '@/components/ui/Input'
import { Modal } from '@/components/ui/Modal'
import { agentLabel } from '@/lib/agentLabel'
import { assignLoopsToBoard } from '@/lib/api/boards'
import { loopsQuery } from '@/lib/api/loops'
import type { Loop } from '@/lib/api/types'
import { keys } from '@/lib/query/keys'

// Show the search box (and pin the list height so typing doesn't bounce the
// modal) only once the list is long enough to need it.
const SEARCH_THRESHOLD = 8

// The one place loops get onto a board: pick existing loops to assign, or hand
// off to LoopModal via onCreateNew for a brand-new one.
export function AddWidgetModal({
  open,
  onClose,
  boardId,
  assignedLoopIds,
  onCreateNew,
  onAssigned,
}: {
  open: boolean
  onClose: () => void
  boardId: string
  // Loops already on this board — offering them again would no-op.
  assignedLoopIds: string[]
  onCreateNew: () => void
  // Reports what was just assigned so the board can scroll it into view.
  onAssigned?: (loopIds: string[]) => void
}) {
  const queryClient = useQueryClient()
  const loops = useQuery(loopsQuery)
  const [selected, setSelected] = useState<string[]>([])
  const [query, setQuery] = useState('')

  const assign = useMutation({
    mutationFn: () => assignLoopsToBoard(boardId, selected),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: keys.boards })
      queryClient.invalidateQueries({ queryKey: keys.boardDetail(boardId) })
      onAssigned?.(selected)
      close()
    },
  })

  const close = () => {
    setSelected([])
    setQuery('')
    assign.reset()
    onClose()
  }

  // Hand off to the loop modal with this picker's state cleared, so reopening
  // it later doesn't resurrect a stale selection.
  const createNew = () => {
    close()
    onCreateNew()
  }

  const toggle = (id: string) =>
    setSelected((current) =>
      current.includes(id) ? current.filter((s) => s !== id) : [...current, id],
    )

  const all = loops.data ?? []
  const available = all.filter(
    (loop) => loop.status !== 'deleted' && !assignedLoopIds.includes(loop.id),
  )
  const hasLoops = available.length > 0
  const searchable = available.length >= SEARCH_THRESHOLD
  const needle = query.trim().toLowerCase()
  const visible = needle
    ? available.filter((loop) => loop.name.toLowerCase().includes(needle))
    : available

  return (
    <Modal
      open={open}
      onClose={close}
      size="md"
      title="Add widget"
      description="Widgets are live tiles kept fresh by loops. Pick loops to put on this board, or start a new one."
      footer={
        <>
          {/* Every exit is disabled while an assign is in flight; otherwise its
              onSuccess close() would yank away whatever the user opened next. */}
          <div className="flex min-w-0 items-center gap-2">
            {assign.isError ? (
              <p role="alert" className="truncate text-[12px] text-danger">
                {(assign.error as Error).message}
              </p>
            ) : null}
          </div>
          <div className="flex shrink-0 items-center gap-1">
            <Button variant="ghost" size="md" disabled={assign.isPending} onClick={close}>
              Cancel
            </Button>
            {hasLoops ? (
              <Button
                variant="primary"
                size="md"
                disabled={selected.length === 0 || assign.isPending}
                onClick={() => assign.mutate()}
              >
                {assign.isPending
                  ? 'Adding…'
                  : selected.length > 1
                    ? `Add ${selected.length} widgets`
                    : 'Add widget'}
              </Button>
            ) : null}
          </div>
        </>
      }
    >
      {loops.isPending ? (
        <p className="text-[13px] text-ink-3">Loading loops…</p>
      ) : loops.isError ? (
        <p role="alert" className="rounded-control bg-danger-soft px-3 py-2 text-[12px] text-danger">
          Couldn’t load loops: {loops.error.message}
        </p>
      ) : !hasLoops ? (
        <DashedCta
          onClick={createNew}
          title={all.length > 0 ? 'Every loop is already on this board' : 'Create your first loop'}
          subtitle="A new loop runs a prompt on a schedule and repaints its widget here on every run."
        />
      ) : (
        <div className={searchable ? 'min-h-72' : undefined}>
          {searchable ? (
            <Input
              type="search"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="Search loops…"
              aria-label="Search loops"
              className="mb-3"
            />
          ) : null}
          <div className="flex flex-col gap-1">
            <NewLoopRow onClick={createNew} disabled={assign.isPending} />
            {visible.length === 0 ? (
              <p className="px-1 py-6 text-center text-[13px] text-ink-3">
                No loops match “{query.trim()}”.
              </p>
            ) : (
              visible.map((loop, index) => (
                <LoopPickRow
                  key={loop.id}
                  loop={loop}
                  delay={Math.min(index + 1, 10) * 0.025}
                  selected={selected.includes(loop.id)}
                  disabled={assign.isPending}
                  onToggle={() => toggle(loop.id)}
                />
              ))
            )}
          </div>
        </div>
      )}
    </Modal>
  )
}

function NewLoopRow({ onClick, disabled }: { onClick: () => void; disabled: boolean }) {
  return (
    <motion.button
      type="button"
      onClick={onClick}
      disabled={disabled}
      initial={{ opacity: 0, y: 4 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.18, ease: 'easeOut' }}
      className="flex items-center gap-2.5 rounded-control bg-bg px-3 py-2 text-left ring-1 ring-border transition-colors duration-150 hover:bg-surface"
    >
      <span className="grid size-4 shrink-0 place-items-center rounded-full bg-primary text-on-primary">
        <Plus size={10} />
      </span>
      <span className="min-w-0 flex-1">
        <span className="block truncate text-[13px] font-medium text-ink">New loop</span>
        <span className="mt-0.5 block truncate text-[11px] text-ink-3">
          Run a prompt on a schedule
        </span>
      </span>
    </motion.button>
  )
}

function LoopPickRow({
  loop,
  delay,
  selected,
  disabled,
  onToggle,
}: {
  loop: Loop
  delay: number
  selected: boolean
  disabled: boolean
  onToggle: () => void
}) {
  const agent = agentLabel(loop.acp_agent || 'jaz')
  return (
    <motion.button
      type="button"
      onClick={onToggle}
      disabled={disabled}
      aria-pressed={selected}
      initial={{ opacity: 0, y: 4 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.18, ease: 'easeOut', delay }}
      className={`flex items-center gap-2.5 rounded-control px-3 py-2 text-left ring-1 transition-colors duration-150 ${
        selected ? 'bg-primary-soft ring-primary/40' : 'bg-bg ring-border hover:bg-surface'
      }`}
    >
      <span
        className={`grid size-4 shrink-0 place-items-center rounded-full ring-1 transition-colors duration-150 ${
          selected ? 'bg-primary text-on-primary ring-primary' : 'bg-bg text-transparent ring-border'
        }`}
      >
        <motion.span
          className="grid place-items-center"
          initial={false}
          animate={
            selected
              ? { scale: 1, opacity: 1, filter: 'blur(0px)' }
              : { scale: 0.25, opacity: 0, filter: 'blur(4px)' }
          }
          transition={{ type: 'spring', duration: 0.3, bounce: 0 }}
        >
          <Check size={10} />
        </motion.span>
      </span>
      <span className="min-w-0 flex-1">
        <span className="flex items-center gap-2">
          <span className="truncate text-[13px] font-medium text-ink">{loop.name}</span>
          {loop.last_run_status === 'error' ? (
            <span className="shrink-0 rounded-full bg-danger-soft px-1.5 py-px text-[10px] font-medium text-danger">
              failed
            </span>
          ) : loop.status === 'paused' ? (
            <span className="shrink-0 rounded-full bg-surface-2 px-1.5 py-px text-[10px] font-medium text-ink-3">
              paused
            </span>
          ) : null}
        </span>
        <span className="mt-0.5 block truncate text-[11px] text-ink-3">{agent}</span>
      </span>
      <span className="shrink-0 text-[11px] tabular-nums text-ink-3">
        {compactSchedule(loop.schedule?.expr ?? '', loop.status === 'paused')}
      </span>
    </motion.button>
  )
}
