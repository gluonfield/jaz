import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { Plus, Settings, SquarePen, Trash2 } from 'lucide-react'
import { AnimatePresence, motion } from 'motion/react'
import { type PointerEvent as ReactPointerEvent, useState } from 'react'
import { BoardModal } from '@/components/boards/BoardModal'
import { LoopModal } from '@/components/loops/LoopModal'
import { IconButton } from '@/components/ui/IconButton'
import { boardsQuery, deleteBoard } from '@/lib/api/boards'
import { loopsQuery } from '@/lib/api/loops'
import { sidebarSessionsQuery, type SessionListItem } from '@/lib/api/sessions'
import type { Board } from '@/lib/api/types'
import { keys } from '@/lib/query/keys'
import { SkeletonRows } from '../ui/Skeleton'
import { LoopRow } from './LoopRow'
import { SessionRow } from './SessionRow'

const SIDEBAR_LOOP_LIMIT = 6
const DEFAULT_PROJECT_LABEL = 'Default'

function SectionLabel({ children, to }: { children: string; to?: '/sessions' }) {
  const className =
    'rounded-full px-2 pb-1 text-[11px] font-semibold tracking-wide text-ink-3 transition-colors duration-150 hover:text-ink'
  if (to) {
    return (
      <Link to={to} className={className} activeOptions={{ exact: true }} activeProps={{ className: 'text-ink!' }}>
        {children}
      </Link>
    )
  }
  return <p className="px-2 pb-1 text-[11px] font-semibold tracking-wide text-ink-3">{children}</p>
}

type SessionProjectGroup = {
  key: string
  label: string
  updatedAt: number
  items: SessionListItem[]
}

function projectName(path: string): string {
  const parts = path.split(/[\\/]+/).filter(Boolean)
  return parts.at(-1) ?? path
}

function isDefaultWorkspace(path: string): boolean {
  const parts = path.split(/[\\/]+/).filter(Boolean)
  return parts.at(-1) === 'default' && parts.at(-2) === 'workspaces'
}

function sessionProject(item: SessionListItem): { key: string; label: string } {
  const cwd =
    item.session.runtime_ref?.project_path?.trim() ||
    item.session.runtime_ref?.cwd?.trim()
  if (!cwd || isDefaultWorkspace(cwd)) return { key: '', label: DEFAULT_PROJECT_LABEL }
  return { key: cwd, label: projectName(cwd) }
}

function sessionUpdatedAt(item: SessionListItem): number {
  const ms = Date.parse(item.session.updated_at)
  return Number.isNaN(ms) ? 0 : ms
}

function groupSessionsByProject(items: SessionListItem[]): SessionProjectGroup[] {
  const groups = new Map<string, SessionProjectGroup>()
  for (const item of items) {
    const project = sessionProject(item)
    const group = groups.get(project.key) ?? {
      key: project.key,
      label: project.label,
      updatedAt: 0,
      items: [],
    }
    group.updatedAt = Math.max(group.updatedAt, sessionUpdatedAt(item))
    group.items.push(item)
    groups.set(project.key, group)
  }
  return [...groups.values()]
    .map((group) => {
      const ids = new Set(group.items.map((item) => item.session.id))
      return {
        ...group,
        items: group.items.map((item) => ({
          ...item,
          child: item.child && Boolean(item.session.parent_id && ids.has(item.session.parent_id)),
        })),
      }
    })
    .sort((a, b) => b.updatedAt - a.updatedAt || a.label.localeCompare(b.label))
}

function LoopsSection() {
  const loops = useQuery(loopsQuery)
  const [creating, setCreating] = useState(false)
  const visibleLoops = loops.data?.slice(0, SIDEBAR_LOOP_LIMIT) ?? []

  return (
    <section>
      <div className="flex items-center justify-between pr-1">
        <Link
          to="/loops"
          className="rounded-full px-2 pb-1 text-[11px] font-semibold tracking-wide text-ink-3 transition-colors duration-150 hover:text-ink"
          activeOptions={{ exact: true }}
          activeProps={{ className: 'text-ink!' }}
        >
          Loops
        </Link>
        <IconButton
          variant="ghost"
          size="xs"
          aria-label="New loop"
          title="New loop"
          onClick={() => setCreating(true)}
          className="-mt-1"
        >
          <Plus size={14} />
        </IconButton>
      </div>
      {loops.isPending ? (
        <SkeletonRows count={2} />
      ) : loops.isError ? (
        <p className="px-2.5 py-1 text-[13px] text-ink-3">Backend unreachable</p>
      ) : visibleLoops.length === 0 ? (
        <button
          type="button"
          onClick={() => setCreating(true)}
          className="rounded-full px-2.5 py-1 text-left text-[13px] text-ink-3 transition-colors duration-150 hover:text-ink"
        >
          Create your first loop
        </button>
      ) : (
        <div className="flex flex-col gap-px">
          <AnimatePresence initial={false} mode="popLayout">
            {visibleLoops.map((loop) => (
              <motion.div
                key={loop.id}
                initial={{ opacity: 0, x: -8 }}
                animate={{ opacity: 1, x: 0 }}
                exit={{ opacity: 0, x: -8 }}
                transition={{ type: 'spring', stiffness: 420, damping: 34 }}
              >
                <LoopRow loop={loop} />
              </motion.div>
            ))}
          </AnimatePresence>
          {loops.data.length > SIDEBAR_LOOP_LIMIT ? (
            <Link
              to="/loops"
              className="mt-1 rounded-full px-2.5 py-1 text-[13px] text-primary transition-colors duration-150 hover:bg-surface-2"
              activeOptions={{ exact: true }}
              activeProps={{ className: 'bg-primary-soft!' }}
            >
              Show all loops
            </Link>
          ) : null}
        </div>
      )}
      <LoopModal open={creating} onClose={() => setCreating(false)} />
    </section>
  )
}

function BoardsSection() {
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
    <section>
      <div className="flex items-center justify-between pr-1">
        <p className="px-2 pb-1 text-[11px] font-semibold tracking-wide text-ink-3">Boards</p>
        <IconButton
          variant="ghost"
          size="xs"
          aria-label="New board"
          title="New board"
          onClick={() => setCreating(true)}
          className="-mt-1"
        >
          <Plus size={14} />
        </IconButton>
      </div>
      {boards.isPending ? (
        <SkeletonRows count={1} />
      ) : boards.isError ? (
        <p className="px-2.5 py-1 text-[13px] text-ink-3">Backend unreachable</p>
      ) : (
        <div className="flex flex-col gap-px">
          {(boards.data ?? []).map((board) => (
            <div key={board.id} className="group relative">
              <Link
                to="/boards/$boardId"
                params={{ boardId: board.id }}
                className="flex w-full items-center rounded-full px-2.5 py-1.5 pr-8 text-left text-[13px] text-ink transition-colors duration-150 hover:bg-surface-2"
                activeProps={{ className: 'bg-primary-soft!' }}
              >
                <span className="min-w-0 flex-1 truncate" title={board.name}>
                  {board.name}
                </span>
              </Link>
              <span className="absolute top-1/2 right-1 -translate-y-1/2 opacity-0 transition-opacity duration-150 group-hover:opacity-100">
                <IconButton
                  variant="ghost"
                  size="xs"
                  aria-label={`Delete board ${board.name}`}
                  title="Delete board"
                  onClick={() => onDelete(board)}
                >
                  <Trash2 size={12} />
                </IconButton>
              </span>
            </div>
          ))}
        </div>
      )}
      <BoardModal open={creating} onClose={() => setCreating(false)} />
    </section>
  )
}

export function Sidebar({
  width,
  resizing,
  onResizeStart,
  onResizeReset,
  onOpenSettings,
}: {
  width: number
  resizing?: boolean
  onResizeStart: (e: ReactPointerEvent) => void
  onResizeReset: () => void
  onOpenSettings: () => void
}) {
  const sessions = useQuery(sidebarSessionsQuery)
  const sessionGroups = groupSessionsByProject(sessions.data ?? [])

  return (
    <aside
      className="sidebar-material relative flex h-full shrink-0 flex-col border-r border-border"
      style={{ width }}
    >
      {/* draggable titlebar strip; traffic lights live here on macOS */}
      <div className="titlebar-drag h-[52px] shrink-0" />

      <nav className="flex min-h-0 flex-1 flex-col gap-5 overflow-y-auto p-3 pt-3">
        <Link
          to="/new"
          className="group flex items-center gap-2 rounded-full px-2.5 py-1.5 text-[13px] font-medium text-ink transition-colors duration-150 hover:bg-surface-2"
          activeProps={{ className: 'bg-primary-soft!' }}
        >
          <SquarePen size={15} className="text-ink-2" />
          <span className="flex-1">New Thread</span>
          <span className="flex items-center gap-0.5 text-[10px] text-ink-3">
            <kbd className="rounded border border-border bg-bg px-1 font-sans">⌘</kbd>
            <kbd className="rounded border border-border bg-bg px-1 font-sans">N</kbd>
          </span>
        </Link>

        <section>
          <SectionLabel to="/sessions">Sessions</SectionLabel>
          {sessions.isPending ? (
            <SkeletonRows count={4} />
          ) : sessions.isError ? (
            <p className="px-2.5 py-1 text-[13px] text-ink-3">Backend unreachable</p>
          ) : sessionGroups.length === 0 ? (
            <p className="px-2.5 py-1 text-[13px] text-ink-3">No sessions yet</p>
          ) : (
            <div className="flex flex-col gap-3">
              {sessionGroups.map((group) => (
                <div key={group.key || 'default'}>
                  <div className="group/project flex items-center justify-between pr-1">
                    <p className="min-w-0 truncate px-2 pb-1 text-[11px] font-medium text-ink-3" title={group.label}>
                      {group.label}
                    </p>
                    <Link
                      to="/new"
                      search={group.key ? { project: group.key } : {}}
                      className="-mt-1 grid size-6 place-items-center rounded-full text-ink-3 opacity-0 transition-colors duration-150 hover:bg-surface-2 hover:text-ink focus-visible:opacity-100 focus-visible:ring-2 focus-visible:ring-primary/50 focus-visible:outline-none group-hover/project:opacity-100"
                      aria-label={`New thread in ${group.label}`}
                      title={`New thread in ${group.label}`}
                    >
                      <SquarePen size={13} />
                    </Link>
                  </div>
                  <div className="flex flex-col gap-px">
                    <AnimatePresence initial={false} mode="popLayout">
                      {group.items.map((item) => (
                        <motion.div
                          key={item.session.id}
                          initial={{ opacity: 0, x: -8 }}
                          animate={{ opacity: 1, x: 0 }}
                          exit={{ opacity: 0, x: -8 }}
                          transition={{ type: 'spring', stiffness: 420, damping: 34 }}
                        >
                          <SessionRow session={item.session} child={item.child} />
                        </motion.div>
                      ))}
                    </AnimatePresence>
                  </div>
                </div>
              ))}
            </div>
          )}
        </section>

        <LoopsSection />

        <BoardsSection />
      </nav>

      <div className="shrink-0 border-t border-border p-3">
        <button
          type="button"
          onClick={onOpenSettings}
          className="group flex w-full items-center gap-2 rounded-full px-2.5 py-1.5 text-[13px] font-medium text-ink transition-colors duration-150 hover:bg-surface-2"
        >
          <Settings size={15} className="text-ink-2" />
          <span className="flex-1 text-left">Settings</span>
        </button>
      </div>

      {/* drag the right edge to resize; double-click resets to the default width */}
      <div
        role="separator"
        aria-orientation="vertical"
        aria-label="Resize sidebar"
        onPointerDown={onResizeStart}
        onDoubleClick={onResizeReset}
        className="group absolute inset-y-0 right-0 z-10 flex w-2 cursor-col-resize touch-none justify-end"
      >
        <span
          className={`h-full w-px transition-colors duration-150 group-hover:bg-primary/40 ${
            resizing ? 'bg-primary/60' : 'bg-transparent'
          }`}
        />
      </div>
    </aside>
  )
}
