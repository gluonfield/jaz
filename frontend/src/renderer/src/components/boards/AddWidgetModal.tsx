import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { AppWindow, Check, Plus } from 'lucide-react'
import { motion } from 'motion/react'
import { useState } from 'react'
import { compactSchedule } from '@/components/loops/schedule'
import { Button } from '@/components/ui/Button'
import { DashedCta } from '@/components/ui/DashedCta'
import { Input } from '@/components/ui/Input'
import { Modal } from '@/components/ui/Modal'
import { agentLabel } from '@/lib/agentLabel'
import { appsQuery, type JazApp } from '@/lib/api/apps'
import { assignAppsToBoard, assignLoopsToBoard } from '@/lib/api/boards'
import { loopsQuery } from '@/lib/api/loops'
import type { Loop } from '@/lib/api/types'
import { keys } from '@/lib/query/keys'

// Show the search box (and pin the list height so typing doesn't bounce the
// modal) only once the list is long enough to need it.
const SEARCH_THRESHOLD = 8

// The one place loops and published apps get onto a board: pick existing loops
// or published apps to assign, or hand off to LoopModal via onCreateNew.
export function AddWidgetModal({
  open,
  onClose,
  boardId,
  assignedLoopIds,
  assignedAppIds = [],
  onCreateNew,
  onAssigned,
}: {
  open: boolean
  onClose: () => void
  boardId: string
  // Loops already on this board — offering them again would no-op.
  assignedLoopIds: string[]
  // Apps already pinned on this board.
  assignedAppIds?: string[]
  onCreateNew: () => void
  // Reports what was just assigned so the board can scroll it into view.
  onAssigned?: (loopIds: string[]) => void
}) {
  const queryClient = useQueryClient()
  const loops = useQuery(loopsQuery)
  const apps = useQuery({ ...appsQuery({ scope: 'all' }), enabled: open })
  const [sourceTab, setSourceTab] = useState<'loops' | 'apps'>('loops')
  const [selected, setSelected] = useState<string[]>([])
  const [selectedApps, setSelectedApps] = useState<string[]>([])
  const [query, setQuery] = useState('')

  const assign = useMutation({
    mutationFn: async () => {
      if (sourceTab === 'apps') {
        await assignAppsToBoard(boardId, selectedApps)
      } else {
        await assignLoopsToBoard(boardId, selected)
      }
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: keys.boards })
      queryClient.invalidateQueries({ queryKey: keys.boardDetail(boardId) })
      if (sourceTab === 'loops') onAssigned?.(selected)
      close()
    },
  })

  const close = () => {
    setSelected([])
    setSelectedApps([])
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
  const toggleApp = (id: string) =>
    setSelectedApps((current) =>
      current.includes(id) ? current.filter((s) => s !== id) : [...current, id],
    )

  const all = loops.data ?? []
  const available = all.filter(
    (loop) => loop.status !== 'deleted' && !assignedLoopIds.includes(loop.id),
  )
  const availableApps = (apps.data ?? []).filter((app) => !assignedAppIds.includes(app.id))
  const hasLoops = available.length > 0
  const hasApps = availableApps.length > 0
  const searchable = (sourceTab === 'loops' ? available.length : availableApps.length) >= SEARCH_THRESHOLD
  const needle = query.trim().toLowerCase()
  const visible = needle
    ? available.filter((loop) => loop.name.toLowerCase().includes(needle))
    : available
  const visibleApps = needle
    ? availableApps.filter((app) => app.title.toLowerCase().includes(needle))
    : availableApps
  const activeCount = sourceTab === 'apps' ? selectedApps.length : selected.length

  return (
    <Modal
      open={open}
      onClose={close}
      size="md"
      title="Add widget or app"
      description="Pin live loop widgets or published Jaz Apps onto this board."
      footer={
        <>
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
            {(sourceTab === 'loops' ? hasLoops : hasApps) ? (
              <Button
                variant="primary"
                size="md"
                disabled={activeCount === 0 || assign.isPending}
                onClick={() => assign.mutate()}
              >
                {assign.isPending
                  ? 'Adding…'
                  : sourceTab === 'apps'
                    ? activeCount > 1
                      ? `Pin ${activeCount} apps`
                      : 'Pin app'
                    : activeCount > 1
                      ? `Add ${activeCount} widgets`
                      : 'Add widget'}
              </Button>
            ) : null}
          </div>
        </>
      }
    >
      <div className="mb-3 flex items-center gap-1 border-b border-border/70">
        {(['loops', 'apps'] as const).map((tabId) => {
          const active = sourceTab === tabId
          return (
            <button
              key={tabId}
              type="button"
              onClick={() => {
                setSourceTab(tabId)
                setQuery('')
              }}
              className={`-mb-px cursor-pointer border-b-2 px-3 py-1.5 text-[12.5px] font-medium transition-colors ${
                active ? 'border-primary text-ink' : 'border-transparent text-ink-3 hover:text-ink'
              }`}
            >
              {tabId === 'loops' ? `Loops (${available.length})` : `Jaz Apps (${availableApps.length})`}
            </button>
          )
        })}
      </div>

      {sourceTab === 'apps' ? (
        apps.isPending ? (
          <p className="text-[13px] text-ink-3">Loading apps…</p>
        ) : apps.isError ? (
          <p role="alert" className="rounded-control bg-danger-soft px-3 py-2 text-[12px] text-danger">
            Couldn’t load apps: {apps.error.message}
          </p>
        ) : !hasApps ? (
          <p className="py-6 text-center text-[13px] text-ink-3">
            No published apps available to pin yet. Publish any artifact or document first.
          </p>
        ) : (
          <div className={searchable ? 'min-h-72' : undefined}>
            {searchable ? (
              <Input
                type="search"
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder="Search apps…"
                aria-label="Search apps"
                className="mb-3"
              />
            ) : null}
            <div className="flex flex-col gap-1">
              {visibleApps.map((app) => (
                <AppPickRow
                  key={app.id}
                  app={app}
                  selected={selectedApps.includes(app.id)}
                  disabled={assign.isPending}
                  onToggle={() => toggleApp(app.id)}
                />
              ))}
            </div>
          </div>
        )
      ) : loops.isPending ? (
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

function AppPickRow({
  app,
  selected,
  disabled,
  onToggle,
}: {
  app: JazApp
  selected: boolean
  disabled: boolean
  onToggle: () => void
}) {
  return (
    <button
      type="button"
      onClick={onToggle}
      disabled={disabled}
      aria-pressed={selected}
      className={`flex cursor-pointer items-center gap-2.5 rounded-control px-3 py-2 text-left ring-1 transition-colors duration-150 ${
        selected ? 'bg-primary-soft ring-primary/40' : 'bg-bg ring-border hover:bg-surface'
      }`}
    >
      <span
        className={`grid size-4 shrink-0 place-items-center rounded-full ring-1 ${
          selected ? 'bg-primary text-on-primary ring-primary' : 'bg-bg text-transparent ring-border'
        }`}
      >
        <Check size={10} />
      </span>
      <AppWindow size={14} className="shrink-0 text-ink-3" />
      <span className="min-w-0 flex-1">
        <span className="block truncate text-[13px] font-medium text-ink">{app.title}</span>
        <span className="block truncate text-[11px] text-ink-3">
          v{app.current_version} · {app.kind} · {app.visibility}
        </span>
      </span>
    </button>
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
  const agent = agentLabel(loop.acp_agent)
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
