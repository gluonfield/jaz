import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { GripVertical, Plus, Settings, SquarePen, Trash2 } from 'lucide-react'
import { AnimatePresence, motion } from 'motion/react'
import { type DragEvent, type PointerEvent as ReactPointerEvent, useState } from 'react'
import { BoardModal } from '@/components/boards/BoardModal'
import { LoopModal } from '@/components/loops/LoopModal'
import { IconButton } from '@/components/ui/IconButton'
import { boardsQuery, deleteBoard } from '@/lib/api/boards'
import { loopsQuery } from '@/lib/api/loops'
import { projectsQuery, reorderProjects, sidebarSessionsQuery, type Project, type SessionListItem } from '@/lib/api/sessions'
import type { Board } from '@/lib/api/types'
import { keys } from '@/lib/query/keys'
import { SkeletonRows } from '../ui/Skeleton'
import { LoopRow } from './LoopRow'
import { SessionRow } from './SessionRow'

const SIDEBAR_LOOP_LIMIT = 6
const PROJECT_SESSION_LIMIT = 5
const DEFAULT_SESSION_LIMIT = 5

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
  items: SessionListItem[]
}

type SessionSections = {
  groups: SessionProjectGroup[]
  ungrouped: SessionListItem[]
}

function sessionProjectPath(item: SessionListItem): string {
  return (
    item.session.runtime_ref?.project_path?.trim() ||
    item.session.runtime_ref?.cwd?.trim() ||
    ''
  )
}

function withLocalChildState(items: SessionListItem[]): SessionListItem[] {
  const ids = new Set(items.map((item) => item.session.id))
  return items.map((item) => ({
    ...item,
    child: item.child && Boolean(item.session.parent_id && ids.has(item.session.parent_id)),
  }))
}

function sessionsBySavedProject(items: SessionListItem[], projects: Project[]): SessionSections {
  const projectByPath = new Map(projects.map((project) => [project.path, project]))
  const groups = new Map<string, SessionProjectGroup>()
  const ungrouped: SessionListItem[] = []

  for (const item of items) {
    const project = projectByPath.get(sessionProjectPath(item))
    if (!project) {
      ungrouped.push(item)
      continue
    }

    const group = groups.get(project.path) ?? {
      key: project.path,
      label: project.name,
      items: [],
    }
    group.items.push(item)
    groups.set(project.path, group)
  }

  return {
    groups: projects
      .map((project) => groups.get(project.path))
      .filter((group): group is SessionProjectGroup => Boolean(group))
      .map((group) => ({ ...group, items: withLocalChildState(group.items) })),
    ungrouped: withLocalChildState(ungrouped),
  }
}

function moveProjectPath(projects: Project[], source: string, target: string): string[] {
  const paths = projects.map((project) => project.path)
  const from = paths.indexOf(source)
  const to = paths.indexOf(target)
  if (from === -1 || to === -1 || from === to) return paths
  const [path] = paths.splice(from, 1)
  paths.splice(paths.indexOf(target), 0, path)
  return paths
}

function SessionRows({ items }: { items: SessionListItem[] }) {
  return (
    <div className="flex flex-col gap-px">
      <AnimatePresence initial={false} mode="popLayout">
        {items.map((item) => (
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
  )
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
  const queryClient = useQueryClient()
  const sessions = useQuery(sidebarSessionsQuery)
  const projects = useQuery(projectsQuery)
  const sessionSections = sessionsBySavedProject(sessions.data ?? [], projects.data ?? [])
  const [expandedProjects, setExpandedProjects] = useState<Set<string>>(() => new Set())
  const [draggingProject, setDraggingProject] = useState('')
  const reorder = useMutation({
    mutationFn: reorderProjects,
    onSuccess: (ordered) => queryClient.setQueryData(keys.projects, ordered),
    onSettled: () => queryClient.invalidateQueries({ queryKey: keys.projects }),
  })
  const hasSessions = sessionSections.groups.length > 0 || sessionSections.ungrouped.length > 0
  const visibleUngrouped = sessionSections.ungrouped.slice(0, DEFAULT_SESSION_LIMIT)
  const expandProject = (key: string) =>
    setExpandedProjects((current) => {
      const next = new Set(current)
      next.add(key)
      return next
    })
  const startProjectDrag = (event: DragEvent, key: string) => {
    setDraggingProject(key)
    event.dataTransfer.effectAllowed = 'move'
    event.dataTransfer.setData('text/plain', key)
  }
  const allowProjectDrop = (event: DragEvent) => {
    if (!draggingProject) return
    event.preventDefault()
    event.dataTransfer.dropEffect = 'move'
  }
  const dropProject = (event: DragEvent, target: string) => {
    event.preventDefault()
    const source = draggingProject || event.dataTransfer.getData('text/plain')
    setDraggingProject('')
    if (!source || source === target || reorder.isPending) return
    reorder.mutate(moveProjectPath(projects.data ?? [], source, target))
  }

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
          {sessions.isPending || projects.isPending ? (
            <SkeletonRows count={4} />
          ) : sessions.isError ? (
            <p className="px-2.5 py-1 text-[13px] text-ink-3">Backend unreachable</p>
          ) : !hasSessions ? (
            <p className="px-2.5 py-1 text-[13px] text-ink-3">No sessions yet</p>
          ) : (
            <div className="flex flex-col gap-3">
              {sessionSections.groups.map((group) => {
                const expanded = expandedProjects.has(group.key)
                const visibleItems = expanded ? group.items : group.items.slice(0, PROJECT_SESSION_LIMIT)
                return (
                  <div
                    key={group.key}
                    onDragOver={allowProjectDrop}
                    onDrop={(event) => dropProject(event, group.key)}
                    className={draggingProject === group.key ? 'opacity-60' : undefined}
                  >
                    <div className="group/project flex items-center justify-between pr-1">
                      <div className="flex min-w-0 items-center">
                        <button
                          type="button"
                          draggable
                          onDragStart={(event) => startProjectDrag(event, group.key)}
                          onDragEnd={() => setDraggingProject('')}
                          className="-ml-1 grid size-5 shrink-0 cursor-grab place-items-center rounded-full text-ink-3 opacity-0 transition-colors duration-150 hover:bg-surface-2 hover:text-ink active:cursor-grabbing focus-visible:opacity-100 focus-visible:ring-2 focus-visible:ring-primary/50 focus-visible:outline-none group-hover/project:opacity-100"
                          aria-label={`Reorder ${group.label}`}
                          title="Drag project"
                        >
                          <GripVertical size={13} />
                        </button>
                        <p className="min-w-0 truncate px-1 pb-1 text-[11px] font-medium text-ink-3" title={group.label}>
                          {group.label}
                        </p>
                      </div>
                      <Link
                        to="/new"
                        search={{ project: group.key }}
                        className="-mt-1 grid size-6 place-items-center rounded-full text-ink-3 opacity-0 transition-colors duration-150 hover:bg-surface-2 hover:text-ink focus-visible:opacity-100 focus-visible:ring-2 focus-visible:ring-primary/50 focus-visible:outline-none group-hover/project:opacity-100"
                        aria-label={`New thread in ${group.label}`}
                        title={`New thread in ${group.label}`}
                      >
                        <SquarePen size={13} />
                      </Link>
                    </div>
                    <SessionRows items={visibleItems} />
                    {!expanded && group.items.length > PROJECT_SESSION_LIMIT ? (
                      <button
                        type="button"
                        onClick={() => expandProject(group.key)}
                        className="mt-1 rounded-full px-2.5 py-1 text-left text-[13px] text-primary transition-colors duration-150 hover:bg-surface-2"
                      >
                        Show more
                      </button>
                    ) : null}
                  </div>
                )
              })}
              {sessionSections.ungrouped.length > 0 ? (
                <div>
                  <SessionRows items={visibleUngrouped} />
                  {sessionSections.ungrouped.length > DEFAULT_SESSION_LIMIT ? (
                    <Link
                      to="/sessions"
                      className="mt-1 block rounded-full px-2.5 py-1 text-[13px] text-primary transition-colors duration-150 hover:bg-surface-2"
                      activeOptions={{ exact: true }}
                      activeProps={{ className: 'bg-primary-soft!' }}
                    >
                      Show all threads
                    </Link>
                  ) : null}
                </div>
              ) : null}
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
