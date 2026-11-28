import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useNavigate } from '@tanstack/react-router'
import { Folder, FolderOpen, Lock, SquarePen, Users } from 'lucide-react'
import { motion, Reorder, type Transition, useDragControls } from 'motion/react'
import { useEffect, useMemo, useState } from 'react'
import { AnimatedList, AnimatedListItem } from '@/components/ui/AnimatedList'
import { CreateProjectDialog } from '@/components/projects/CreateProjectDialog'
import { Collapse } from '@/components/ui/Collapse'
import { HoverCardGroup } from '@/components/ui/HoverCard'
import { SkeletonRows } from '@/components/ui/Skeleton'
import { spacesQuery, type CloudSpace } from '@/lib/api/cloudSpaces'
import {
  projectsQuery,
  reorderProjects,
  sidebarSessionsQuery,
} from '@/lib/api/sessions'
import { useShowModelIcons } from '@/lib/appearance'
import { useCloudSession, useIsCloudOnly, useIsCloudViewer } from '@/lib/cloudSession'
import { modalDialogOpen } from '@/lib/dom/modal'
import { useMetaHeld } from '@/lib/hooks/useMetaHeld'
import { useWindowEvent } from '@/lib/hooks/useWindowEvent'
import { keys } from '@/lib/query/keys'
import { SessionRow } from './SessionRow'
import { SpaceModal } from './SpaceModal'
import {
  SidebarOrganizationMenu,
  type SidebarViewState,
  useSidebarView,
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
  type SessionListItem,
} from './sidebarSessionModel'

const COLLAPSED_PROJECTS_KEY = 'jaz.sidebar.collapsedProjects'
const MORE_ACTION_CLASS =
  'flex h-[30px] items-center rounded-lg pr-2.5 text-[13px] text-ink-3 opacity-80 transition-[background-color,color,opacity] duration-150 hover:bg-list-hover hover:text-ink hover:opacity-100 max-sm:h-11 max-sm:pr-3 max-sm:text-[15px]'
const SECTION_LABEL_CLASS = 'text-[13px] font-medium text-ink-3 max-sm:text-[15px]'
const GROUP_LABEL_CLASS = 'text-[13px] font-medium text-ink max-sm:text-[15px]'
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
  reserveIconSpace = true,
}: {
  items: SessionListItem[]
  shortcutByID: Map<string, number>
  shortcutMode: boolean
  reserveIconSpace?: boolean
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
              reserveIconSpace={reserveIconSpace}
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
        className={`flex h-[30px] items-center px-2.5 max-sm:h-11 max-sm:px-3 ${SECTION_LABEL_CLASS}`}
      >
        Pinned
      </p>
      <SessionRows items={items} shortcutByID={shortcutByID} shortcutMode={shortcutMode} />
    </div>
  )
}

function ProjectGroupIcon({ group }: { group: Extract<SessionDisplayBlock, { kind: 'project' }>['group'] }) {
  if (group.kind === 'bucket') return null
  const space = group.space
  const Icon = space
    ? space.visibility === 'private'
      ? Lock
      : space.visibility === 'team'
        ? Users
        : Folder
    : Folder
  return (
    <span className="grid size-[18px] shrink-0 place-items-center">
      <Icon size={15} className="text-ink-2" />
    </span>
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
  const navigate = useNavigate()
  const isViewer = useIsCloudViewer()
  const dragControls = useDragControls()
  const { group, items, collapsed } = block
  const isLocalProject = group.kind === 'local'
  const isCloudProject = group.kind === 'cloud' && Boolean(group.space)
  const isBucket = group.kind === 'bucket'
  const hasThreads = group.items.length > 0
  const rowHoverActionClass = `grid size-6 place-items-center rounded-md text-ink-3 transition-colors duration-150 hover:bg-surface-2 hover:text-ink focus-visible:opacity-100 group-hover/project:opacity-100 [@media(hover:none)]:opacity-100 ${hasThreads ? 'opacity-0' : ''}`
  const handleTap = () => {
    if (hasThreads) {
      onToggle()
    } else if (isCloudProject && group.space) {
      void navigate({ to: '/spaces/$spaceId', params: { spaceId: group.space.id } })
    } else if (isLocalProject) {
      void navigate({ to: '/new', search: { project: group.key } })
    }
  }

  return (
    <Reorder.Item
      as="div"
      value={block.key}
      layout="position"
      transition={ROW_SPRING}
      dragListener={false}
      dragControls={dragControls}
      onDragEnd={isLocalProject ? onReorderEnd : undefined}
    >
      <div className="group/project flex h-[30px] items-center justify-between rounded-lg pr-1 transition-colors duration-150 hover:bg-list-hover max-sm:h-11">
        <motion.button
          type="button"
          onPointerDown={isLocalProject ? (event) => dragControls.start(event) : undefined}
          onTap={handleTap}
          aria-expanded={hasThreads ? !collapsed : undefined}
          aria-label={
            hasThreads
              ? `${collapsed ? 'Expand' : 'Collapse'} ${group.label}`
              : isCloudProject
                ? `Open project ${group.label}`
                : `New task in ${group.label}`
          }
          className={`flex h-full min-w-0 flex-1 touch-none items-center gap-2 rounded-lg px-2.5 text-left outline-none focus-visible:ring-2 focus-visible:ring-primary/40 ${
            isLocalProject ? 'cursor-grab active:cursor-grabbing' : 'cursor-pointer'
          }`}
        >
          <ProjectGroupIcon group={group} />
          <span
            className={`min-w-0 truncate ${isBucket ? SECTION_LABEL_CLASS : GROUP_LABEL_CLASS}`}
            title={group.label}
          >
            {group.label}
          </span>
        </motion.button>
        {isLocalProject ? (
          <Link
            to="/new"
            search={{ project: group.key }}
            className={rowHoverActionClass}
            aria-label={`New task in ${group.label}`}
            title={`New task in ${group.label}`}
          >
            <SquarePen size={13} />
          </Link>
        ) : isCloudProject && group.space ? (
          <div className="flex items-center gap-0.5">
            <Link
              to="/spaces/$spaceId"
              params={{ spaceId: group.space.id }}
              className={rowHoverActionClass}
              aria-label={`Open project ${group.label}`}
              title={`Open ${group.label} (sources & instructions)`}
            >
              <FolderOpen size={13} />
            </Link>
            {!isViewer ? (
              <Link
                to="/new"
                search={{ space: group.space.id }}
                className={rowHoverActionClass}
                aria-label={`New task in ${group.label}`}
                title={`New task in ${group.label}`}
              >
                <SquarePen size={13} />
              </Link>
            ) : null}
          </div>
        ) : null}
      </div>
      {hasThreads ? (
        <Collapse open={!collapsed}>
          <SessionRows
            items={items}
            shortcutByID={shortcutByID}
            shortcutMode={shortcutMode}
            reserveIconSpace={!isBucket}
          />
          {items.length < group.items.length ? (
            <button
              type="button"
              onClick={onShowMore}
              className={`${MORE_ACTION_CLASS} ${isBucket ? 'pl-2.5 max-sm:pl-3' : 'pl-9 max-sm:pl-10'}`}
            >
              Show More
            </button>
          ) : null}
        </Collapse>
      ) : null}
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
      className="not-first:mt-3"
    >
      <p
        className={`flex h-[30px] items-center px-2.5 max-sm:h-11 max-sm:px-3 ${SECTION_LABEL_CLASS}`}
      >
        Recents
      </p>
      <RecentSessionList
        items={block.items}
        total={block.total}
        onShowMore={onShowMore}
        shortcutByID={shortcutByID}
        shortcutMode={shortcutMode}
      />
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
      className="flex flex-col gap-px"
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
      <SessionRows
        items={items}
        shortcutByID={shortcutByID}
        shortcutMode={shortcutMode}
        reserveIconSpace={false}
      />
      {items.length < total ? (
        <button type="button" onClick={onShowMore} className={`${MORE_ACTION_CLASS} pl-2.5 max-sm:pl-3`}>
          Show more
        </button>
      ) : null}
    </div>
  )
}

export function SidebarSessions({ open }: { open: boolean }) {
  const queryClient = useQueryClient()
  const navigate = useNavigate()
  const cloud = useCloudSession()
  const hasCloud = cloud.active
  const cloudOnly = useIsCloudOnly()
  const isViewer = useIsCloudViewer()
  const sessions = useQuery(sidebarSessionsQuery)
  const cloudSpaces = useQuery({ ...spacesQuery, enabled: hasCloud })
  const projects = useQuery({ ...projectsQuery, enabled: !cloudOnly })
  const [view, setView] = useSidebarView()
  const [visibleRecentCount, setVisibleRecentCount] = useState(SESSION_PAGE_SIZE)
  const [expandedProjects, setExpandedProjects] = useState<Set<string>>(() => new Set())
  const [showAllUngrouped, setShowAllUngrouped] = useState(false)
  const [collapsedProjects, setCollapsedProjects] = useState(storedCollapsedProjects)
  const [dragOrder, setDragOrder] = useState<SessionBlockKey[] | null>(null)
  const [createProjectOpen, setCreateProjectOpen] = useState(false)
  useEffect(() => storeCollapsedProjects(collapsedProjects), [collapsedProjects])
  useEffect(() => {
    setDragOrder(null)
    setVisibleRecentCount(SESSION_PAGE_SIZE)
    setShowAllUngrouped(false)
  }, [cloud.activeWorkspaceId, hasCloud])

  const showEnvironmentOption = useMemo(() => {
    if (hasCloud && !cloudOnly) return true
    const list = sessions.data ?? []
    const hasCloudThread = list.some((s) => s.backend === 'cloud')
    const hasLocalThread = list.some((s) => s.backend !== 'cloud')
    return hasCloudThread && hasLocalThread
  }, [cloudOnly, hasCloud, sessions.data])

  const sections = useMemo(
    () => sidebarSessionSections(
      sessions.data ?? [],
      cloudOnly ? [] : (projects.data ?? []),
      {
        spaces: hasCloud ? (cloudSpaces.data ?? []) : [],
        groupBy: view.groupBy,
        sortBy: view.sortBy,
        show: view.show,
      },
    ),
    [cloudOnly, cloudSpaces.data, hasCloud, projects.data, sessions.data, view.groupBy, view.show, view.sortBy],
  )
  const groups = useMemo(
    () => (view.groupBy === 'project' ? applySessionDragOrder(sections.groups, dragOrder) : sections.groups),
    [dragOrder, sections.groups, view.groupBy],
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
  const hasAnySessions = (sessions.data?.length ?? 0) > 0
  const hasVisibleSessions = sections.pinnedItems.length > 0 || sections.recentItems.length > 0
  const shortcutItems = useMemo(
    () => [
      ...sections.pinnedItems,
      ...(view.groupBy !== 'none'
        ? blocks.flatMap((block) => block.kind === 'project' && block.collapsed ? [] : block.items)
        : recentPage.items),
    ].slice(0, 9),
    [blocks, recentPage.items, sections.pinnedItems, view.groupBy],
  )
  const shortcutByID = useMemo(
    () => new Map(shortcutItems.map((item, index) => [item.session.id, index + 1])),
    [shortcutItems],
  )
  const shortcutMode = useThreadShortcuts(shortcutItems, open && hasVisibleSessions)
  const reorder = useMutation({
    mutationFn: reorderProjects,
    onSuccess: (ordered) => queryClient.setQueryData(keys.projects, ordered),
    onSettled: () => queryClient.invalidateQueries({ queryKey: keys.projects }),
  })

  const changeView = (next: SidebarViewState) => {
    setView(next)
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

  if (
    sessions.isPending ||
    (view.groupBy === 'project' && ((!cloudOnly && projects.isPending) || (hasCloud && cloudSpaces.isPending)))
  ) {
    return <SkeletonRows count={4} />
  }
  if (sessions.isError && !sessions.data) {
    return <p className="px-2.5 py-1 text-[13px] text-ink-3">Backend unreachable</p>
  }

  const canCreateProject = !isViewer && (hasCloud || !cloudOnly)

  return (
    <HoverCardGroup>
      <section className="flex shrink-0 flex-col gap-3">
        <PinnedSessions
          items={sections.pinnedItems}
          shortcutByID={shortcutByID}
          shortcutMode={shortcutMode}
        />
        <div>
          <SidebarOrganizationMenu
            view={view}
            onChange={changeView}
            showEnvironmentOption={showEnvironmentOption}
            onCreateProject={canCreateProject ? () => setCreateProjectOpen(true) : undefined}
          />
          {view.groupBy !== 'none' ? (
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
          {!hasAnySessions && blocks.length === 0 ? (
            <p className="px-2.5 py-1 text-[13px] text-ink-3">No chats yet</p>
          ) : hasAnySessions && !hasVisibleSessions && view.show !== 'all' ? (
            <p className="px-2.5 py-1 text-[13px] text-ink-3">No matching chats</p>
          ) : null}
        </div>
        {createProjectOpen ? (
          hasCloud ? (
            <SpaceModal
              onClose={() => setCreateProjectOpen(false)}
              onCreated={(created) => {
                queryClient.setQueryData<CloudSpace[]>(keys.cloudSpaces, (prev) => [
                  ...(prev ?? []),
                  created,
                ])
                void navigate({ to: '/spaces/$spaceId', params: { spaceId: created.id } })
              }}
            />
          ) : (
            <CreateProjectDialog
              onClose={() => setCreateProjectOpen(false)}
              onCreated={(project) => {
                setCreateProjectOpen(false)
                void navigate({ to: '/new', search: { project: project.path } })
              }}
            />
          )
        ) : null}
      </section>
    </HoverCardGroup>
  )
}
