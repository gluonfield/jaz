import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useNavigate } from '@tanstack/react-router'
import { ChevronDown, Folder, Pin, SquarePen } from 'lucide-react'
import { motion, Reorder, type Transition, useDragControls } from 'motion/react'
import { useEffect, useMemo, useState } from 'react'
import { AnimatedList, AnimatedListItem } from '@/components/ui/AnimatedList'
import { Collapse } from '@/components/ui/Collapse'
import { SkeletonRows } from '@/components/ui/Skeleton'
import {
  projectsQuery,
  reorderProjects,
  sidebarSessionsQuery,
  type SessionListItem,
} from '@/lib/api/sessions'
import { useShowModelIcons } from '@/lib/appearance'
import { modalDialogOpen } from '@/lib/dom/modal'
import { useMetaHeld } from '@/lib/hooks/useMetaHeld'
import { useWindowEvent } from '@/lib/hooks/useWindowEvent'
import { keys } from '@/lib/query/keys'
import { SessionRow } from './SessionRow'
import {
  SidebarOrganizationMenu,
  type SidebarOrganization,
  useSidebarOrganization,
} from './SidebarOrganizationMenu'
import {
  applySidebarProjectOrder,
  applySessionDragOrder,
  sessionDisplayBlocks,
  sessionPage,
  sidebarSessionSections,
  SESSION_PAGE_SIZE,
  type SessionBlockKey,
  type SessionDisplayBlock,
} from './sidebarSessionModel'

const COLLAPSED_PROJECTS_KEY = 'jaz.sidebar.collapsedProjects'
const MORE_ACTION_CLASS =
  'flex h-[30px] items-center rounded-full pl-9 pr-2.5 text-[13px] text-ink-3 opacity-80 transition-[background-color,color,opacity] duration-150 hover:bg-list-hover hover:text-ink hover:opacity-100 max-sm:h-11 max-sm:pl-10 max-sm:pr-3 max-sm:text-[15px]'
const SECTION_HEADING_CLASS = 'text-[13px] font-semibold text-ink max-sm:text-[15px]'
const ROW_SPRING: Transition = { type: 'spring', stiffness: 420, damping: 34 }

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
  shortcutByID: Map<string, number>
  shortcutMode: boolean
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
              shortcutIndex={shortcutByID.get(item.session.id)}
              shortcutMode={shortcutMode}
              showRuntimeBadge={showRuntimeBadge}
            />
          </AnimatedListItem>
        ))}
      </AnimatedList>
    </div>
  )
}

function PinnedSessions({
  items,
  shortcutByID,
  shortcutMode,
}: {
  items: SessionListItem[]
  shortcutByID: Map<string, number>
  shortcutMode: boolean
}) {
  if (!items.length) return null
  return (
    <div>
      <p
        className={`flex h-[30px] items-center gap-2 px-2.5 max-sm:h-11 max-sm:gap-2.5 max-sm:px-3 ${SECTION_HEADING_CLASS}`}
      >
        <span className="grid size-[18px] shrink-0 place-items-center">
          <Pin size={14} className="text-ink-2" />
        </span>
        Pinned
      </p>
      <SessionRows items={items} shortcutByID={shortcutByID} shortcutMode={shortcutMode} />
    </div>
  )
}

function ProjectGroup({
  block,
  onToggle,
  onShowMore,
  onReorderEnd,
  shortcutByID,
  shortcutMode,
}: {
  block: Extract<SessionDisplayBlock, { kind: 'project' }>
  onToggle: () => void
  onShowMore: () => void
  onReorderEnd: () => void
  shortcutByID: Map<string, number>
  shortcutMode: boolean
}) {
  const dragControls = useDragControls()
  const { group, items, collapsed } = block
  return (
    <Reorder.Item
      as="div"
      value={block.key}
      layout="position"
      transition={ROW_SPRING}
      dragListener={false}
      dragControls={dragControls}
      onDragEnd={onReorderEnd}
    >
      <div className="group/project flex h-[30px] items-center justify-between rounded-full pr-1 transition-colors duration-150 hover:bg-list-hover max-sm:h-11">
        <motion.button
          type="button"
          onPointerDown={(event) => dragControls.start(event)}
          onTap={onToggle}
          aria-expanded={!collapsed}
          aria-label={`${collapsed ? 'Expand' : 'Collapse'} ${group.label}`}
          className="flex h-full min-w-0 flex-1 cursor-grab touch-none items-center gap-2 rounded-full px-2.5 text-left outline-none active:cursor-grabbing focus-visible:ring-2 focus-visible:ring-primary/40"
        >
          <span className="grid size-[18px] shrink-0 place-items-center">
            <Folder size={15} className="text-ink-2" />
          </span>
          <span className={`min-w-0 truncate ${SECTION_HEADING_CLASS}`} title={group.label}>
            {group.label}
          </span>
          <ChevronDown
            size={13}
            className={`-ml-1 shrink-0 text-ink-3 transition-[color,rotate] duration-150 ease-out group-hover/project:text-ink ${collapsed ? '-rotate-90' : ''}`}
            aria-hidden
          />
        </motion.button>
        <Link
          to="/new"
          search={{ project: group.key }}
          className="grid size-6 place-items-center rounded-full text-ink-3 opacity-0 transition-colors duration-150 hover:bg-surface-2 hover:text-ink focus-visible:opacity-100 group-hover/project:opacity-100"
          aria-label={`New task in ${group.label}`}
          title={`New task in ${group.label}`}
        >
          <SquarePen size={13} />
        </Link>
      </div>
      <Collapse open={!collapsed}>
        <SessionRows items={items} shortcutByID={shortcutByID} shortcutMode={shortcutMode} />
        {items.length < group.items.length ? (
          <button type="button" onClick={onShowMore} className={MORE_ACTION_CLASS}>
            Show more
          </button>
        ) : null}
      </Collapse>
    </Reorder.Item>
  )
}

function UngroupedSessions({
  block,
  onShowMore,
  shortcutByID,
  shortcutMode,
}: {
  block: Extract<SessionDisplayBlock, { kind: 'ungrouped' }>
  onShowMore: () => void
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
      <p
        className={`flex h-[30px] items-center gap-2 px-2.5 max-sm:h-11 max-sm:gap-2.5 max-sm:px-3 ${SECTION_HEADING_CLASS}`}
      >
        <span className="grid size-[18px] shrink-0 place-items-center">
          <Folder size={15} className="text-ink-3" />
        </span>
        No project
      </p>
      <SessionRows items={block.items} shortcutByID={shortcutByID} shortcutMode={shortcutMode} />
      {block.items.length < block.total ? (
        <button type="button" onClick={onShowMore} className={MORE_ACTION_CLASS}>
          Show more
        </button>
      ) : null}
    </Reorder.Item>
  )
}

function ProjectSessionList({
  blocks,
  onReorder,
  onReorderEnd,
  onToggle,
  onExpand,
  onExpandUngrouped,
  shortcutByID,
  shortcutMode,
}: {
  blocks: SessionDisplayBlock[]
  onReorder: (order: SessionBlockKey[]) => void
  onReorderEnd: () => void
  onToggle: (key: string) => void
  onExpand: (key: string) => void
  onExpandUngrouped: () => void
  shortcutByID: Map<string, number>
  shortcutMode: boolean
}) {
  if (!blocks.length) return null
  return (
    <Reorder.Group
      as="div"
      axis="y"
      values={blocks.map((block) => block.key)}
      onReorder={onReorder}
      className="mt-1 flex flex-col gap-3"
    >
      {blocks.map((block) =>
        block.kind === 'project' ? (
          <ProjectGroup
            key={block.key}
            block={block}
            onToggle={() => onToggle(block.group.key)}
            onShowMore={() => onExpand(block.group.key)}
            onReorderEnd={onReorderEnd}
            shortcutByID={shortcutByID}
            shortcutMode={shortcutMode}
          />
        ) : (
          <UngroupedSessions
            key={block.key}
            block={block}
            onShowMore={onExpandUngrouped}
            shortcutByID={shortcutByID}
            shortcutMode={shortcutMode}
          />
        ),
      )}
    </Reorder.Group>
  )
}

function RecentSessionList({
  items,
  total,
  onShowMore,
  shortcutByID,
  shortcutMode,
}: {
  items: SessionListItem[]
  total: number
  onShowMore: () => void
  shortcutByID: Map<string, number>
  shortcutMode: boolean
}) {
  if (!items.length) return null
  return (
    <div>
      <SessionRows items={items} shortcutByID={shortcutByID} shortcutMode={shortcutMode} />
      {items.length < total ? (
        <button type="button" onClick={onShowMore} className={MORE_ACTION_CLASS}>
          Show more
        </button>
      ) : null}
    </div>
  )
}

export function SidebarSessions({ open }: { open: boolean }) {
  const queryClient = useQueryClient()
  const sessions = useQuery(sidebarSessionsQuery)
  const projects = useQuery(projectsQuery)
  const [organization, setOrganization] = useSidebarOrganization()
  const [visibleRecentCount, setVisibleRecentCount] = useState(SESSION_PAGE_SIZE)
  const [expandedProjects, setExpandedProjects] = useState<Set<string>>(() => new Set())
  const [showAllUngrouped, setShowAllUngrouped] = useState(false)
  const [collapsedProjects, setCollapsedProjects] = useState(storedCollapsedProjects)
  const [dragOrder, setDragOrder] = useState<SessionBlockKey[] | null>(null)
  useEffect(() => storeCollapsedProjects(collapsedProjects), [collapsedProjects])

  const sections = useMemo(
    () => sidebarSessionSections(sessions.data ?? [], projects.data ?? []),
    [projects.data, sessions.data],
  )
  const groups = useMemo(
    () => applySessionDragOrder(sections.groups, dragOrder),
    [dragOrder, sections.groups],
  )
  const blocks = useMemo(
    () => sessionDisplayBlocks(
      groups,
      sections.ungrouped,
      expandedProjects,
      showAllUngrouped,
      collapsedProjects,
    ),
    [collapsedProjects, expandedProjects, groups, sections.ungrouped, showAllUngrouped],
  )
  const recentPage = useMemo(
    () => sessionPage(sections.recentItems, visibleRecentCount),
    [sections.recentItems, visibleRecentCount],
  )
  const hasSessions = (sessions.data?.length ?? 0) > 0
  const shortcutItems = useMemo(
    () => [
      ...sections.pinnedItems,
      ...(organization === 'project'
        ? blocks.flatMap((block) => block.kind === 'project' && block.collapsed ? [] : block.items)
        : recentPage.items),
    ].slice(0, 9),
    [blocks, organization, recentPage.items, sections.pinnedItems],
  )
  const shortcutByID = useMemo(
    () => new Map(shortcutItems.map((item, index) => [item.session.id, index + 1])),
    [shortcutItems],
  )
  const shortcutMode = useThreadShortcuts(shortcutItems, open && hasSessions)
  const reorder = useMutation({
    mutationFn: reorderProjects,
    onSuccess: (ordered) => queryClient.setQueryData(keys.projects, ordered),
    onSettled: () => queryClient.invalidateQueries({ queryKey: keys.projects }),
  })

  const changeOrganization = (next: SidebarOrganization) => {
    setOrganization(next)
    setDragOrder(null)
  }
  const expandProject = (key: string) => {
    setExpandedProjects((current) => new Set(current).add(key))
  }
  const toggleProject = (key: string) => {
    setCollapsedProjects((current) => {
      const next = new Set(current)
      if (next.has(key)) next.delete(key)
      else next.add(key)
      return next
    })
  }
  const commitReorder = () => {
    if (!dragOrder) return
    const all = projects.data ?? []
    const next = applySidebarProjectOrder(all, dragOrder)
    queryClient.setQueryData(keys.projects, next)
    setDragOrder(null)
    reorder.mutate(next.map((project) => project.path))
  }

  if (sessions.isPending || organization === 'project' && projects.isPending) {
    return <SkeletonRows count={4} />
  }
  if (sessions.isError) {
    return <p className="px-2.5 py-1 text-[13px] text-ink-3">Backend unreachable</p>
  }

  return (
    <section className="flex shrink-0 flex-col gap-3">
      <PinnedSessions
        items={sections.pinnedItems}
        shortcutByID={shortcutByID}
        shortcutMode={shortcutMode}
      />
      <div>
        <SidebarOrganizationMenu organization={organization} onChange={changeOrganization} />
        {organization === 'project' ? (
          <ProjectSessionList
            blocks={blocks}
            onReorder={setDragOrder}
            onReorderEnd={commitReorder}
            onToggle={toggleProject}
            onExpand={expandProject}
            onExpandUngrouped={() => setShowAllUngrouped(true)}
            shortcutByID={shortcutByID}
            shortcutMode={shortcutMode}
          />
        ) : (
          <RecentSessionList
            {...recentPage}
            onShowMore={() => setVisibleRecentCount((count) => count + SESSION_PAGE_SIZE)}
            shortcutByID={shortcutByID}
            shortcutMode={shortcutMode}
          />
        )}
        {!hasSessions ? (
          <p className="px-2.5 py-1 text-[13px] text-ink-3">No sessions yet</p>
        ) : null}
      </div>
    </section>
  )
}
