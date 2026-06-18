import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useNavigate } from '@tanstack/react-router'
import { GripVertical, Plus, Settings, SquarePen, Trash2 } from 'lucide-react'
import { AnimatePresence, motion, Reorder, type Transition, useDragControls } from 'motion/react'
import { type PointerEvent as ReactPointerEvent, useEffect, useMemo, useState } from 'react'
import { BoardModal } from '@/components/boards/BoardModal'
import { LoopModal } from '@/components/loops/LoopModal'
import { IconButton } from '@/components/ui/IconButton'
import { KeyboardShortcut } from '@/components/ui/KeyboardShortcut'
import { UpdatePanel } from '@/components/update/UpdatePanel'
import { boardsQuery, deleteBoard } from '@/lib/api/boards'
import { loopsQuery } from '@/lib/api/loops'
import { projectsQuery, reorderProjects, sidebarSessionsQuery, type Project, type SessionListItem } from '@/lib/api/sessions'
import { useWindowEvent } from '@/lib/hooks/useWindowEvent'
import type { Board } from '@/lib/api/types'
import { keys } from '@/lib/query/keys'
import { SkeletonRows } from '../ui/Skeleton'
import { LoopRow } from './LoopRow'
import { SessionRow } from './SessionRow'

const SIDEBAR_LOOP_LIMIT = 6
const PROJECT_SESSION_LIMIT = 5
const DEFAULT_SESSION_LIMIT = 5

const ROW_SPRING: Transition = { type: 'spring', stiffness: 420, damping: 34 }

type SessionProjectGroup = {
  key: string
  label: string
  items: SessionListItem[]
}

type SessionSections = {
  groups: SessionProjectGroup[]
  ungrouped: SessionListItem[]
}

type SessionDisplayBlock =
  | { kind: 'pinned'; key: 'pinned'; label: 'Pinned'; items: SessionListItem[] }
  | { kind: 'project'; key: string; group: SessionProjectGroup; items: SessionListItem[] }
  | { kind: 'ungrouped'; key: 'ungrouped'; items: SessionListItem[]; total: number }

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

// Groups missing from the drag order (e.g. refetched mid-drag) keep their
// relative order at the end.
function applyDragOrder(groups: SessionProjectGroup[], order: string[] | null): SessionProjectGroup[] {
  if (!order) return groups
  const rank = new Map(order.map((key, index) => [key, index]))
  return [...groups].sort((a, b) => (rank.get(a.key) ?? order.length) - (rank.get(b.key) ?? order.length))
}

function visibleProjectItems(group: SessionProjectGroup, expanded: boolean): SessionListItem[] {
  return expanded ? group.items : group.items.slice(0, PROJECT_SESSION_LIMIT)
}

function sessionDisplayBlocks(
  pinnedItems: SessionListItem[],
  groups: SessionProjectGroup[],
  ungrouped: SessionListItem[],
  expandedProjects: Set<string>,
): SessionDisplayBlock[] {
  const blocks: SessionDisplayBlock[] = []
  if (pinnedItems.length) blocks.push({ kind: 'pinned', key: 'pinned', label: 'Pinned', items: pinnedItems })
  for (const group of groups) {
    blocks.push({
      kind: 'project',
      key: group.key,
      group,
      items: visibleProjectItems(group, expandedProjects.has(group.key)),
    })
  }
  if (ungrouped.length) {
    blocks.push({
      kind: 'ungrouped',
      key: 'ungrouped',
      items: ungrouped.slice(0, DEFAULT_SESSION_LIMIT),
      total: ungrouped.length,
    })
  }
  return blocks
}

function modalOpen(): boolean {
  return Boolean(document.querySelector('[role="dialog"][aria-modal="true"]'))
}

function useThreadShortcuts(items: SessionListItem[], enabled: boolean): boolean {
  const navigate = useNavigate()
  const [shortcutMode, setShortcutMode] = useState(false)

  useEffect(() => {
    if (!enabled) setShortcutMode(false)
  }, [enabled])

  useWindowEvent(
    'keydown',
    (event) => {
      if (modalOpen()) {
        setShortcutMode(false)
        return
      }
      if (event.key === 'Meta' || event.metaKey) setShortcutMode(true)
      if (!event.metaKey || event.defaultPrevented || event.altKey || event.ctrlKey) return
      if (!/^[1-9]$/.test(event.key)) return
      const item = items[Number(event.key) - 1]
      if (!item) return
      event.preventDefault()
      navigate({ to: '/sessions/$sessionId', params: { sessionId: item.session.id } })
    },
    enabled,
  )

  useWindowEvent('keyup', (event) => {
    if (event.key === 'Meta') setShortcutMode(false)
  }, enabled)
  useWindowEvent('blur', () => setShortcutMode(false), enabled)

  return enabled && shortcutMode
}

function SessionRows({
  items,
  shortcutByID,
  shortcutMode,
}: {
  items: SessionListItem[]
  shortcutByID?: Map<string, number>
  shortcutMode?: boolean
}) {
  return (
    <div className="flex flex-col gap-px">
      <AnimatePresence initial={false} mode="popLayout">
        {items.map((item) => (
          <motion.div
            key={item.session.id}
            layout="position"
            initial={{ opacity: 0, x: -8 }}
            animate={{ opacity: 1, x: 0 }}
            exit={{ opacity: 0, x: -8 }}
            transition={ROW_SPRING}
          >
            <SessionRow
              session={item.session}
              child={item.child}
              shortcutIndex={shortcutByID?.get(item.session.id)}
              shortcutMode={shortcutMode}
            />
          </motion.div>
        ))}
      </AnimatePresence>
    </div>
  )
}

function ProjectGroup({
  group,
  items,
  onExpand,
  onReorderEnd,
  shortcutByID,
  shortcutMode,
}: {
  group: SessionProjectGroup
  items: SessionListItem[]
  onExpand: () => void
  onReorderEnd: () => void
  shortcutByID: Map<string, number>
  shortcutMode: boolean
}) {
  const dragControls = useDragControls()
  return (
    <Reorder.Item
      as="div"
      value={group.key}
      layout="position"
      transition={ROW_SPRING}
      dragListener={false}
      dragControls={dragControls}
      onDragEnd={onReorderEnd}
    >
      <div className="group/project flex items-center justify-between pr-1">
        <div className="flex min-w-0 flex-1 items-center">
          {/* -ml-3 hangs the grip in the nav's left padding so the project
              name stays aligned with the Loops/Boards labels */}
          <button
            type="button"
            onPointerDown={(event) => dragControls.start(event)}
            className="-ml-3 grid size-5 shrink-0 cursor-grab touch-none place-items-center rounded-full text-ink-3 opacity-0 transition-colors duration-150 hover:bg-surface-2 hover:text-ink active:cursor-grabbing focus-visible:opacity-100 group-hover/project:opacity-100"
            aria-label={`Reorder ${group.label}`}
            title="Drag to reorder"
          >
            <GripVertical size={13} />
          </button>
          <p className="min-w-0 truncate pb-1 text-[11px] font-medium text-ink-3" title={group.label}>
            {group.label}
          </p>
        </div>
        <Link
          to="/new"
          search={{ project: group.key }}
          className="-mt-1 grid size-6 place-items-center rounded-full text-ink-3 opacity-0 transition-colors duration-150 hover:bg-surface-2 hover:text-ink focus-visible:opacity-100 group-hover/project:opacity-100"
          aria-label={`New thread in ${group.label}`}
          title={`New thread in ${group.label}`}
        >
          <SquarePen size={13} />
        </Link>
      </div>
      <SessionRows items={items} shortcutByID={shortcutByID} shortcutMode={shortcutMode} />
      {items.length < group.items.length ? (
        <button
          type="button"
          onClick={onExpand}
          className="mt-1 rounded-full px-2.5 py-1 text-left text-[13px] text-primary transition-colors duration-150 hover:bg-surface-2"
        >
          Show more
        </button>
      ) : null}
    </Reorder.Item>
  )
}

function SessionsSection({ open }: { open: boolean }) {
  const queryClient = useQueryClient()
  const sessions = useQuery(sidebarSessionsQuery)
  const projects = useQuery(projectsQuery)
  const pinnedItems = withLocalChildState((sessions.data ?? []).filter((item) => item.session.pinned))
  const sections = sessionsBySavedProject(
    (sessions.data ?? []).filter((item) => !item.session.pinned),
    projects.data ?? [],
  )
  const [expandedProjects, setExpandedProjects] = useState<Set<string>>(() => new Set())
  // Group keys in their live mid-drag order; null while no drag is happening.
  const [dragOrder, setDragOrder] = useState<string[] | null>(null)
  const reorder = useMutation({
    mutationFn: reorderProjects,
    onSuccess: (ordered) => queryClient.setQueryData(keys.projects, ordered),
    onSettled: () => queryClient.invalidateQueries({ queryKey: keys.projects }),
  })
  const groups = applyDragOrder(sections.groups, dragOrder)
  const blocks = useMemo(
    () => sessionDisplayBlocks(pinnedItems, groups, sections.ungrouped, expandedProjects),
    [expandedProjects, groups, pinnedItems, sections.ungrouped],
  )
  const hasSessions = blocks.length > 0
  const shortcutItems = useMemo(
    () => blocks.flatMap((block) => block.items).slice(0, 9),
    [blocks],
  )
  const shortcutByID = useMemo(
    () => new Map(shortcutItems.map((item, index) => [item.session.id, index + 1])),
    [shortcutItems],
  )
  const shortcutMode = useThreadShortcuts(shortcutItems, open && hasSessions)
  const pinnedBlock = blocks.find((block): block is Extract<SessionDisplayBlock, { kind: 'pinned' }> =>
    block.kind === 'pinned',
  )
  const projectBlocks = blocks.filter((block): block is Extract<SessionDisplayBlock, { kind: 'project' }> =>
    block.kind === 'project',
  )
  const ungroupedBlock = blocks.find((block): block is Extract<SessionDisplayBlock, { kind: 'ungrouped' }> =>
    block.kind === 'ungrouped',
  )

  const expandProject = (key: string) =>
    setExpandedProjects((current) => {
      const next = new Set(current)
      next.add(key)
      return next
    })

  const commitReorder = () => {
    if (!dragOrder) return
    // Permute the dragged (visible) groups within the full saved project
    // list; projects with no sidebar sessions keep their slots.
    const all = projects.data ?? []
    const byPath = new Map(all.map((project) => [project.path, project]))
    const queue = dragOrder
      .map((key) => byPath.get(key))
      .filter((project): project is Project => Boolean(project))
    const draggedSet = new Set(queue.map((project) => project.path))
    const next = all.map((project) => (draggedSet.has(project.path) ? queue.shift()! : project))
    queryClient.setQueryData(keys.projects, next)
    setDragOrder(null)
    reorder.mutate(next.map((project) => project.path))
  }

  return (
    <section>
      {sessions.isPending || projects.isPending ? (
        <SkeletonRows count={4} />
      ) : sessions.isError ? (
        <p className="px-2.5 py-1 text-[13px] text-ink-3">Backend unreachable</p>
      ) : !hasSessions ? (
        <p className="px-2.5 py-1 text-[13px] text-ink-3">No sessions yet</p>
      ) : (
        <div className="flex flex-col gap-3">
          {pinnedBlock ? (
            <div>
              <p className="px-2 pb-1 text-[11px] font-semibold tracking-wide text-ink-3">
                {pinnedBlock.label}
              </p>
              <SessionRows items={pinnedBlock.items} shortcutByID={shortcutByID} shortcutMode={shortcutMode} />
            </div>
          ) : null}
          {projectBlocks.length > 0 ? (
            <Reorder.Group
              as="div"
              axis="y"
              values={projectBlocks.map((block) => block.key)}
              onReorder={setDragOrder}
              className="flex flex-col gap-3"
            >
              {projectBlocks.map((block) => (
                <ProjectGroup
                  key={block.key}
                  group={block.group}
                  items={block.items}
                  onExpand={() => expandProject(block.key)}
                  onReorderEnd={commitReorder}
                  shortcutByID={shortcutByID}
                  shortcutMode={shortcutMode}
                />
              ))}
            </Reorder.Group>
          ) : null}
          {ungroupedBlock ? (
            <div>
              <SessionRows items={ungroupedBlock.items} shortcutByID={shortcutByID} shortcutMode={shortcutMode} />
              {ungroupedBlock.total > DEFAULT_SESSION_LIMIT ? (
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
                layout="position"
                initial={{ opacity: 0, x: -8 }}
                animate={{ opacity: 1, x: 0 }}
                exit={{ opacity: 0, x: -8 }}
                transition={ROW_SPRING}
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
  open,
  width,
  resizing,
  onResizeStart,
  onResizeReset,
  onOpenSettings,
}: {
  open: boolean
  width: number
  resizing?: boolean
  onResizeStart: (e: ReactPointerEvent) => void
  onResizeReset: () => void
  onOpenSettings: () => void
}) {
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
          <KeyboardShortcut value="N" />
        </Link>

        <SessionsSection open={open} />

        <LoopsSection />

        <BoardsSection />
      </nav>

      <div className="flex shrink-0 flex-col gap-1.5 border-t border-border p-3">
        <UpdatePanel />
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
