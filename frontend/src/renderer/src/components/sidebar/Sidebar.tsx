import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, type LinkComponentProps, useNavigate } from '@tanstack/react-router'
import { ChevronDown, GripVertical, Inbox, LayoutDashboard, Repeat, Settings, SquarePen } from 'lucide-react'
import { Reorder, type Transition, useDragControls } from 'motion/react'
import { type PointerEvent as ReactPointerEvent, type ReactNode, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { ConnectionFooterButton } from '@/components/connection/ConnectionFooterButton'
import { AnimatedList, AnimatedListItem } from '@/components/ui/AnimatedList'
import { Collapse } from '@/components/ui/Collapse'
import { KeyboardShortcut } from '@/components/ui/KeyboardShortcut'
import { UpdatePanel } from '@/components/update/UpdatePanel'
import { feedQuery } from '@/lib/api/feed'
import { activeRunStatus, loopsQuery, TONE_DOT } from '@/lib/api/loops'
import { projectsQuery, reorderProjects, sidebarSessionsQuery, type Project, type SessionListItem } from '@/lib/api/sessions'
import { useShowModelIcons } from '@/lib/appearance'
import { modalDialogOpen } from '@/lib/dom/modal'
import { useMetaHeld } from '@/lib/hooks/useMetaHeld'
import { useWindowEvent } from '@/lib/hooks/useWindowEvent'
import { keys } from '@/lib/query/keys'
import { SkeletonRows } from '../ui/Skeleton'
import { SessionRow } from './SessionRow'

const PROJECT_SESSION_LIMIT = 5
const DEFAULT_SESSION_LIMIT = 5
const COLLAPSED_PROJECTS_KEY = 'jaz.sidebar.collapsedProjects'
const MORE_ACTION_CLASS =
  'flex h-8 items-center rounded-full px-2.5 text-[13px] text-ink-3 opacity-80 transition-[background-color,color,opacity] duration-150 hover:bg-surface-2 hover:text-ink hover:opacity-100 max-sm:h-11 max-sm:px-3 max-sm:text-[15px]'

// Group headings (Pinned, project names) share one anchor style so they stay
// stronger than the chat rows beneath them.
const SECTION_HEADING_CLASS = 'pb-1 text-[13px] font-semibold text-ink max-sm:text-[15px]'

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
  | { kind: 'project'; key: string; group: SessionProjectGroup; items: SessionListItem[]; collapsed: boolean }
  | { kind: 'ungrouped'; key: 'ungrouped'; items: SessionListItem[]; total: number }

function explicitSessionProjectPath(item: SessionListItem): string {
  return item.session.runtime_ref?.project_path?.trim() || ''
}

function sessionProjectPath(item: SessionListItem): string {
  return explicitSessionProjectPath(item) || item.session.runtime_ref?.cwd?.trim() || ''
}

function sidebarProjectPath(
  item: SessionListItem,
  byID: Map<string, SessionListItem>,
  childProjectByParentID: Map<string, string>,
): string {
  const parent = item.child && item.session.parent_id ? byID.get(item.session.parent_id) : undefined
  if (parent) {
    return (
      explicitSessionProjectPath(parent) ||
      explicitSessionProjectPath(item) ||
      sessionProjectPath(parent) ||
      sessionProjectPath(item)
    )
  }
  return explicitSessionProjectPath(item) || childProjectByParentID.get(item.session.id) || sessionProjectPath(item)
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
  const byID = new Map(items.map((item) => [item.session.id, item]))
  const childProjectByParentID = new Map<string, string>()
  for (const item of items) {
    const parentID = item.session.parent_id
    const path = explicitSessionProjectPath(item) || sessionProjectPath(item)
    if (item.child && parentID && path && !childProjectByParentID.has(parentID)) {
      childProjectByParentID.set(parentID, path)
    }
  }
  const groups = new Map<string, SessionProjectGroup>()
  const ungrouped: SessionListItem[] = []

  for (const item of items) {
    const project = projectByPath.get(sidebarProjectPath(item, byID, childProjectByParentID))
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

function storedCollapsedProjects(): Set<string> {
  try {
    const paths: unknown = JSON.parse(localStorage.getItem(COLLAPSED_PROJECTS_KEY) ?? '[]')
    return new Set(Array.isArray(paths) ? paths.filter((path): path is string => typeof path === 'string') : [])
  } catch {
    return new Set()
  }
}

function storeCollapsedProjects(paths: Set<string>): void {
  try {
    localStorage.setItem(COLLAPSED_PROJECTS_KEY, JSON.stringify([...paths]))
  } catch {
    // This preference should never break the sidebar when storage is unavailable.
  }
}

function projectSessionSlice(group: SessionProjectGroup, showAll: boolean): SessionListItem[] {
  return showAll ? group.items : group.items.slice(0, PROJECT_SESSION_LIMIT)
}

function sessionListItemTime(item: SessionListItem): number {
  const ms = Date.parse(item.session.last_attention_at || item.session.updated_at)
  return Number.isNaN(ms) ? 0 : ms
}

function sessionListItemsTime(items: SessionListItem[]): number {
  return Math.max(0, ...items.map(sessionListItemTime))
}

function sessionDisplayBlocks(
  pinnedItems: SessionListItem[],
  groups: SessionProjectGroup[],
  ungrouped: SessionListItem[],
  showAllProjects: Set<string>,
  collapsedProjects: Set<string>,
): SessionDisplayBlock[] {
  const blocks: SessionDisplayBlock[] = []
  if (pinnedItems.length) blocks.push({ kind: 'pinned', key: 'pinned', label: 'Pinned', items: pinnedItems })
  const projectBlocks: SessionDisplayBlock[] = groups.map((group) => ({
    kind: 'project',
    key: group.key,
    group,
    items: projectSessionSlice(group, showAllProjects.has(group.key)),
    collapsed: collapsedProjects.has(group.key),
  }))
  if (ungrouped.length) {
    const ungroupedBlock: SessionDisplayBlock = {
      kind: 'ungrouped',
      key: 'ungrouped',
      items: ungrouped.slice(0, DEFAULT_SESSION_LIMIT),
      total: ungrouped.length,
    }
    const time = sessionListItemsTime(ungrouped)
    const index = groups.findIndex((group) => time > sessionListItemsTime(group.items))
    if (index === -1) projectBlocks.push(ungroupedBlock)
    else projectBlocks.splice(index, 0, ungroupedBlock)
  }
  blocks.push(...projectBlocks)
  return blocks
}

function useThreadShortcuts(items: SessionListItem[], enabled: boolean): boolean {
  const navigate = useNavigate()
  const metaHeld = useMetaHeld(enabled)

  useWindowEvent(
    'keydown',
    (event) => {
      if (!event.metaKey || event.defaultPrevented || event.altKey || event.ctrlKey) return
      if (!/^[1-9]$/.test(event.key) || modalDialogOpen()) return
      const item = items[Number(event.key) - 1]
      if (!item) return
      event.preventDefault()
      navigate({ to: '/sessions/$sessionId', params: { sessionId: item.session.id } })
    },
    enabled,
  )

  return metaHeld
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
  const showRuntimeBadge = useShowModelIcons()
  return (
    <div className="flex flex-col gap-px">
      <AnimatedList>
        {items.map((item) => (
          <AnimatedListItem
            key={item.session.id}
            initial={{ opacity: 0, x: -8 }}
            animate={{ opacity: 1, x: 0 }}
          >
            <SessionRow
              session={item.session}
              child={item.child}
              shortcutIndex={shortcutByID?.get(item.session.id)}
              shortcutMode={shortcutMode}
              showRuntimeBadge={showRuntimeBadge}
            />
          </AnimatedListItem>
        ))}
      </AnimatedList>
    </div>
  )
}

function ProjectGroup({
  group,
  items,
  collapsed,
  onToggle,
  onShowMore,
  onReorderEnd,
  shortcutByID,
  shortcutMode,
}: {
  group: SessionProjectGroup
  items: SessionListItem[]
  collapsed: boolean
  onToggle: () => void
  onShowMore: () => void
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
      <div className="group/project flex h-8 items-center justify-between pr-1 max-sm:h-11">
        <div className="flex min-w-0 flex-1 items-center">
          {/* -ml-3 hangs the grip in the nav's left padding so the project
              name stays aligned with the nav labels above */}
          <button
            type="button"
            onPointerDown={(event) => dragControls.start(event)}
            className="-ml-3 grid size-5 shrink-0 cursor-grab touch-none place-items-center rounded-full text-ink-3 opacity-0 transition-colors duration-150 hover:bg-surface-2 hover:text-ink active:cursor-grabbing focus-visible:opacity-100 group-hover/project:opacity-100"
            aria-label={`Reorder ${group.label}`}
            title="Drag to reorder"
          >
            <GripVertical size={13} />
          </button>
          <button
            type="button"
            onClick={onToggle}
            aria-expanded={!collapsed}
            aria-label={`${collapsed ? 'Expand' : 'Collapse'} ${group.label}`}
            className="flex h-full min-w-0 flex-1 items-center rounded-full text-left outline-none focus-visible:ring-2 focus-visible:ring-primary/40"
          >
            <span className={`min-w-0 truncate ${SECTION_HEADING_CLASS}`} title={group.label}>
              {group.label}
            </span>
            <ChevronDown
              size={13}
              className={`-mt-1 ml-1 shrink-0 text-ink-3 transition-[color,transform] duration-150 ease-out group-hover/project:text-ink ${collapsed ? '-rotate-90' : ''}`}
              aria-hidden
            />
          </button>
        </div>
        <Link
          to="/new"
          search={{ project: group.key }}
          className="-mt-1 grid size-6 place-items-center rounded-full text-ink-3 opacity-0 transition-colors duration-150 hover:bg-surface-2 hover:text-ink focus-visible:opacity-100 group-hover/project:opacity-100"
          aria-label={`New task in ${group.label}`}
          title={`New task in ${group.label}`}
        >
          <SquarePen size={13} />
        </Link>
      </div>
      <Collapse open={!collapsed}>
        <SessionRows items={items} shortcutByID={shortcutByID} shortcutMode={shortcutMode} />
        {items.length < group.items.length ? (
          <button
            type="button"
            onClick={onShowMore}
            className={MORE_ACTION_CLASS}
          >
            Show more
          </button>
        ) : null}
      </Collapse>
    </Reorder.Item>
  )
}

function UngroupedSessionsBlock({
  block,
  shortcutByID,
  shortcutMode,
}: {
  block: Extract<SessionDisplayBlock, { kind: 'ungrouped' }>
  shortcutByID: Map<string, number>
  shortcutMode: boolean
}) {
  return (
    <Reorder.Item
      as="div"
      value={block.key}
      layout="position"
      transition={ROW_SPRING}
      dragListener={false}
    >
      <SessionRows items={block.items} shortcutByID={shortcutByID} shortcutMode={shortcutMode} />
      {block.total > DEFAULT_SESSION_LIMIT ? (
        <Link
          to="/sessions"
          className={MORE_ACTION_CLASS}
          activeOptions={{ exact: true }}
          activeProps={{ className: 'bg-primary-soft! opacity-100!' }}
        >
          Show all threads
        </Link>
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
  const [showAllProjects, setShowAllProjects] = useState<Set<string>>(() => new Set())
  const [collapsedProjects, setCollapsedProjects] = useState(storedCollapsedProjects)
  useEffect(() => storeCollapsedProjects(collapsedProjects), [collapsedProjects])
  // Group keys in their live mid-drag order; null while no drag is happening.
  const [dragOrder, setDragOrder] = useState<string[] | null>(null)
  const reorder = useMutation({
    mutationFn: reorderProjects,
    onSuccess: (ordered) => queryClient.setQueryData(keys.projects, ordered),
    onSettled: () => queryClient.invalidateQueries({ queryKey: keys.projects }),
  })
  const groups = applyDragOrder(sections.groups, dragOrder)
  const blocks = useMemo(
    () => sessionDisplayBlocks(pinnedItems, groups, sections.ungrouped, showAllProjects, collapsedProjects),
    [collapsedProjects, groups, pinnedItems, sections.ungrouped, showAllProjects],
  )
  const hasSessions = blocks.length > 0
  const shortcutItems = useMemo(
    () =>
      blocks
        .flatMap((block) => block.kind === 'project' && block.collapsed ? [] : block.items)
        .slice(0, 9),
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
  const sessionBlocks = blocks.filter((block) => block.kind !== 'pinned')

  const expandProject = (key: string) =>
    setShowAllProjects((current) => {
      const next = new Set(current)
      next.add(key)
      return next
    })

  const toggleProject = (key: string) =>
    setCollapsedProjects((current) => {
      const next = new Set(current)
      if (next.has(key)) next.delete(key)
      else next.add(key)
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
    <section className="shrink-0">
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
              <p className={`px-2 ${SECTION_HEADING_CLASS}`}>{pinnedBlock.label}</p>
              <SessionRows items={pinnedBlock.items} shortcutByID={shortcutByID} shortcutMode={shortcutMode} />
            </div>
          ) : null}
          {sessionBlocks.length > 0 ? (
            <Reorder.Group
              as="div"
              axis="y"
              values={sessionBlocks.map((block) => block.key)}
              onReorder={setDragOrder}
              className="flex flex-col gap-3"
            >
              {sessionBlocks.map((block) =>
                block.kind === 'project' ? (
                  <ProjectGroup
                    key={block.key}
                    group={block.group}
                    items={block.items}
                    collapsed={block.collapsed}
                    onToggle={() => toggleProject(block.key)}
                    onShowMore={() => expandProject(block.key)}
                    onReorderEnd={commitReorder}
                    shortcutByID={shortcutByID}
                    shortcutMode={shortcutMode}
                  />
                ) : (
                  <UngroupedSessionsBlock
                    key={block.key}
                    block={block}
                    shortcutByID={shortcutByID}
                    shortcutMode={shortcutMode}
                  />
                ),
              )}
            </Reorder.Group>
          ) : null}
        </div>
      )}
    </section>
  )
}

// Same row metrics as SessionRow (h-8 / max-sm:h-11, 1px gaps) so the rail
// reads as one rhythm from New task down through the chat lists.
const NAV_LINK_CLASS =
  'group flex h-8 items-center gap-2 rounded-full px-2.5 text-[13px] font-medium text-ink transition-colors duration-150 hover:bg-surface-2 max-sm:h-11 max-sm:px-3 max-sm:text-[15px]'

function NavLink({
  to,
  icon,
  label,
  badge,
}: {
  to: LinkComponentProps['to']
  icon: ReactNode
  label: string
  badge?: ReactNode
}) {
  return (
    <Link to={to} className={NAV_LINK_CLASS} activeProps={{ className: 'bg-primary-soft!' }}>
      <span className="grid size-[18px] shrink-0 place-items-center">{icon}</span>
      <span className="flex-1">{label}</span>
      {badge}
    </Link>
  )
}

function FeedLink() {
  const feed = useQuery(feedQuery)
  const count = feed.data?.length ?? 0
  return (
    <NavLink
      to="/feed"
      icon={<Inbox size={15} className="text-ink-2 max-sm:size-[18px]" />}
      label="Feed"
      badge={
        count > 0 ? (
          // A div (not span): the sidebar force-colors span text to --color-ink in
          // dark mode, which would erase the count on the bg-ink bubble.
          <div className="inline-flex h-[14px] min-w-[14px] items-center justify-center rounded-full bg-ink px-1 text-[9px] font-semibold leading-none tabular-nums text-bg">
            {count > 99 ? '99+' : count}
          </div>
        ) : null
      }
    />
  )
}

function LoopsLink() {
  const loops = useQuery(loopsQuery)
  const running = loops.data?.some((loop) => activeRunStatus(loop.last_run_status))
  const failed = loops.data?.some((loop) => loop.last_run_status === 'error')
  return (
    <NavLink
      to="/loops"
      icon={<Repeat size={15} className="text-ink-2 max-sm:size-[18px]" />}
      label="Loops"
      badge={
        running ? (
          <span title="A loop is running" className={`size-1.5 shrink-0 rounded-full ${TONE_DOT.running}`} />
        ) : failed ? (
          <span title="A loop failed" className={`size-1.5 shrink-0 rounded-full ${TONE_DOT.failed}`} />
        ) : null
      }
    />
  )
}

export function Sidebar({
  open,
  width,
  mobile = false,
  onDismiss,
  resizing,
  onResizeStart,
  onResizeReset,
  onOpenSettings,
  onOpenConnect,
}: {
  open: boolean
  width: number
  mobile?: boolean
  onDismiss?: () => void
  resizing?: boolean
  onResizeStart: (e: ReactPointerEvent) => void
  onResizeReset: () => void
  onOpenSettings: () => void
  onOpenConnect: () => void
}) {
  const navRef = useRef<HTMLElement | null>(null)
  const [navEdge, setNavEdge] = useState({ scrollable: false, scrolled: false })
  const updateNavEdge = useCallback(() => {
    const nav = navRef.current
    const scrollable = Boolean(nav && nav.scrollHeight - nav.clientHeight > 1)
    const scrolled = Boolean(scrollable && nav && nav.scrollTop > 1)
    setNavEdge((current) =>
      current.scrollable === scrollable && current.scrolled === scrolled
        ? current
        : { scrollable, scrolled },
    )
  }, [])

  useEffect(() => {
    updateNavEdge()
    const nav = navRef.current
    if (!nav) return

    const resizeObserver = new ResizeObserver(updateNavEdge)
    resizeObserver.observe(nav)
    const mutationObserver = new MutationObserver(updateNavEdge)
    mutationObserver.observe(nav, { childList: true, subtree: true })
    window.addEventListener('resize', updateNavEdge)
    const frame = window.requestAnimationFrame(updateNavEdge)

    return () => {
      window.cancelAnimationFrame(frame)
      window.removeEventListener('resize', updateNavEdge)
      mutationObserver.disconnect()
      resizeObserver.disconnect()
    }
  }, [updateNavEdge])

  const showNavEdge = navEdge.scrollable && navEdge.scrolled

  return (
    <aside
      // Phone: the drawer is full-screen (CSS overrides the inline column width).
      // Empty-space taps and navigation links dismiss it — including re-tapping
      // the already-active session, which doesn't change the route — while
      // in-place action buttons (pin/archive/rename) keep it open.
      onClick={
        mobile && onDismiss
          ? (e) => {
              if (!(e.target as HTMLElement).closest('button, input, textarea')) onDismiss()
            }
          : undefined
      }
      className="sidebar-material relative flex h-full shrink-0 flex-col border-r border-border max-sm:w-full!"
      style={{ width }}
    >
      {/* draggable titlebar strip; traffic lights live here on macOS. On a phone
          there is no window to drag, and the drag region would swallow the taps
          that should dismiss the full-screen drawer, so drop it there. */}
      <div className={`h-[52px] shrink-0 ${mobile ? '' : 'titlebar-drag'}`} />

      <div className="flex shrink-0 flex-col px-3 pb-px max-sm:px-4">
        <NavLink
          to="/new"
          icon={<SquarePen size={15} className="text-ink-2 max-sm:size-[18px]" />}
          label="New task"
          badge={<KeyboardShortcut value="N" className="max-sm:hidden" />}
        />
      </div>

      <div
        aria-hidden
        className={`pointer-events-none relative z-[1] h-0 shrink-0 transition-opacity duration-150 ${
          showNavEdge ? 'opacity-100' : 'opacity-0'
        }`}
      >
        <div className="h-px bg-border/70" />
        <div className="absolute inset-x-0 top-px h-5 bg-gradient-to-b from-[var(--sidebar-material-bg)] to-transparent" />
      </div>

      <nav
        ref={navRef}
        onScroll={updateNavEdge}
        className="scrollbar-quiet flex min-h-0 flex-1 flex-col gap-5 overflow-y-auto px-3 max-sm:gap-6 max-sm:px-4"
      >
        <div className="flex flex-col gap-px">
          <FeedLink />
          <LoopsLink />
          <NavLink
            to="/boards"
            icon={<LayoutDashboard size={15} className="text-ink-2 max-sm:size-[18px]" />}
            label="Boards"
          />
        </div>

        <SessionsSection open={open} />
      </nav>

      <div className="flex shrink-0 flex-col gap-0.5 border-t border-border px-3 py-1.5">
        <UpdatePanel />
        <ConnectionFooterButton onOpenConnect={onOpenConnect} />
        <button
          type="button"
          onClick={onOpenSettings}
          className="group flex w-full items-center gap-2 rounded-full px-2.5 py-1 text-[13px] font-medium text-ink transition-colors duration-150 hover:bg-surface-2 max-sm:px-3 max-sm:py-2 max-sm:text-[15px]"
        >
          <Settings size={15} className="text-ink-2 max-sm:size-[18px]" />
          <span className="flex-1 text-left">Settings</span>
        </button>
      </div>

      {/* drag the right edge to resize; double-click resets to the default
          width. A phone sidebar is full-screen, so there is nothing to resize. */}
      <div
        role="separator"
        aria-orientation="vertical"
        aria-label="Resize sidebar"
        onPointerDown={onResizeStart}
        onDoubleClick={onResizeReset}
        className="group absolute inset-y-0 right-0 z-10 flex w-2 cursor-col-resize touch-none justify-end max-sm:hidden"
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
