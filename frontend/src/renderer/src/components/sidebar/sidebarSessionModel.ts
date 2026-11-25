import type { CloudSpace } from '@/lib/api/cloudSpaces'
import type { Project } from '@/lib/api/sessions'
import type { Session } from '@/lib/api/types'

export const SESSION_PAGE_SIZE = 50
const PROJECT_SESSION_LIMIT = 5
const UNGROUPED_BLOCK_KEY = 'ungrouped'
export type SessionBlockKey = `project:${string}` | typeof UNGROUPED_BLOCK_KEY

export type SidebarGroupBy = 'project' | 'date' | 'status' | 'environment' | 'none'
export type SidebarSortBy = 'updated' | 'created'
export type SidebarShowFilter = 'all' | 'active' | 'unread'

export interface SidebarFilterOptions {
  spaces?: CloudSpace[]
  groupBy?: SidebarGroupBy
  sortBy?: SidebarSortBy
  show?: SidebarShowFilter
  now?: Date
}

export interface SessionListItem {
  session: Session
  child: boolean
}

export type SessionProjectGroup = {
  key: string
  label: string
  items: SessionListItem[]
  kind?: 'local' | 'cloud' | 'bucket'
  space?: CloudSpace
}

export type SessionSections = {
  pinnedItems: SessionListItem[]
  recentItems: SessionListItem[]
  groups: SessionProjectGroup[]
  ungrouped: SessionListItem[]
}

export type SessionDisplayBlock =
  | { kind: 'project'; key: `project:${string}`; group: SessionProjectGroup; items: SessionListItem[]; collapsed: boolean }
  | { kind: 'ungrouped'; key: typeof UNGROUPED_BLOCK_KEY; items: SessionListItem[]; total: number }

export function sessionProjectBlockKey(path: string): `project:${string}` {
  return `project:${path}`
}

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

function sessionsBySavedProject(
  items: SessionListItem[],
  projects: Project[],
  spaces: CloudSpace[],
  keepEmpty: boolean,
) {
  const cloudGroups = new Map<string, SessionProjectGroup>(spaces.map((space) => [
    space.id,
    { key: space.id, label: space.name, items: [], kind: 'cloud', space },
  ]))
  const localGroups = new Map<string, SessionProjectGroup>(projects.map((project) => [
    project.path,
    { key: project.path, label: project.name, items: [], kind: 'local' },
  ]))
  const byID = new Map(items.map((item) => [item.session.id, item]))
  const childProjectByParentID = new Map<string, string>()
  for (const item of items) {
    const parentID = item.session.parent_id
    const path = explicitSessionProjectPath(item) || sessionProjectPath(item)
    if (item.child && parentID && path && !childProjectByParentID.has(parentID)) {
      childProjectByParentID.set(parentID, path)
    }
  }
  const ungrouped: SessionListItem[] = []

  for (const item of items) {
    const spaceID = item.session.space_id?.trim()
    if (spaceID) {
      const cloudGroup = cloudGroups.get(spaceID)
      if (cloudGroup) {
        cloudGroup.items.push(item)
        continue
      }
    }
    const localGroup = localGroups.get(sidebarProjectPath(item, byID, childProjectByParentID))
    if (localGroup) {
      localGroup.items.push(item)
      continue
    }
    ungrouped.push(item)
  }

  return {
    groups: [...cloudGroups.values(), ...localGroups.values()]
      .filter((group) => keepEmpty || group.items.length > 0)
      .map((group) => ({ ...group, items: withLocalChildState(group.items) })),
    ungrouped: withLocalChildState(ungrouped),
  }
}

function sessionTime(session: Session, sortBy: SidebarSortBy = 'updated'): number {
  const raw = sortBy === 'created'
    ? (session.created_at || session.updated_at || session.last_attention_at)
    : (session.last_attention_at || session.updated_at)
  const ms = Date.parse(raw)
  return Number.isNaN(ms) ? 0 : ms
}

function matchesShowFilter(session: Session, show: SidebarShowFilter): boolean {
  if (show === 'unread') return Boolean(session.unread)
  if (show === 'active') return session.status === 'running' || Boolean(session.unread)
  return true
}

function groupSessionsForDisplay(sessions: Session[], sortBy: SidebarSortBy = 'updated'): SessionListItem[] {
  const ids = new Set(sessions.map((session) => session.id))
  const children = new Map<string, Session[]>()
  const roots: Session[] = []

  for (const session of sessions) {
    if (session.parent_id && session.parent_id !== session.id && ids.has(session.parent_id)) {
      children.set(session.parent_id, [...(children.get(session.parent_id) ?? []), session])
    } else {
      roots.push(session)
    }
  }

  const groupTimes = new Map<string, number>()
  const groupTime = (session: Session, visiting = new Set<string>()): number => {
    const cached = groupTimes.get(session.id)
    if (cached !== undefined) return cached
    if (visiting.has(session.id)) return sessionTime(session, sortBy)

    visiting.add(session.id)
    let latest = sessionTime(session, sortBy)
    for (const child of children.get(session.id) ?? []) {
      latest = Math.max(latest, groupTime(child, visiting))
    }
    visiting.delete(session.id)
    groupTimes.set(session.id, latest)
    return latest
  }

  const compareGroups = (a: Session, b: Session): number =>
    groupTime(b) - groupTime(a) || sessionTime(b, sortBy) - sessionTime(a, sortBy) || a.id.localeCompare(b.id)

  const ordered: SessionListItem[] = []
  const emitted = new Set<string>()
  const append = (session: Session, isChild: boolean) => {
    if (emitted.has(session.id)) return
    emitted.add(session.id)
    ordered.push({ session, child: isChild })
    for (const sub of [...(children.get(session.id) ?? [])].sort(compareGroups)) {
      append(sub, true)
    }
  }

  for (const root of [...roots].sort(compareGroups)) append(root, false)
  for (const session of [...sessions].sort(compareGroups)) append(session, false)
  return ordered
}

function startOfDay(date: Date): number {
  return new Date(date.getFullYear(), date.getMonth(), date.getDate()).getTime()
}

function dateBucket(ms: number, now: Date): { key: string; label: string } {
  const todayStart = startOfDay(now)
  const dayMs = 24 * 60 * 60 * 1000
  if (ms >= todayStart) return { key: 'date:today', label: 'Today' }
  if (ms >= todayStart - dayMs) return { key: 'date:yesterday', label: 'Yesterday' }
  if (ms >= todayStart - 7 * dayMs) return { key: 'date:7d', label: 'Previous 7 days' }
  if (ms >= todayStart - 30 * dayMs) return { key: 'date:30d', label: 'Previous 30 days' }
  return { key: 'date:older', label: 'Older' }
}

function statusBucket(session: Session): { key: string; label: string } {
  if (session.status === 'running') return { key: 'status:running', label: 'Running' }
  if (session.unread) return { key: 'status:unread', label: 'Unread' }
  if (session.status === 'error' || session.status === 'interrupted') {
    return { key: 'status:attention', label: 'Needs attention' }
  }
  return { key: 'status:idle', label: 'Idle' }
}

function groupByBuckets(
  items: SessionListItem[],
  order: Array<{ key: string; label: string }>,
  bucketOf: (item: SessionListItem) => { key: string; label: string },
): SessionProjectGroup[] {
  const buckets = new Map<string, SessionProjectGroup>(
    order.map((entry) => [entry.key, { key: entry.key, label: entry.label, items: [], kind: 'bucket' }]),
  )
  for (const item of items) {
    const target = bucketOf(item)
    const bucket = buckets.get(target.key)
    if (bucket) bucket.items.push(item)
  }
  return [...buckets.values()]
    .filter((bucket) => bucket.items.length > 0)
    .map((bucket) => ({ ...bucket, items: withLocalChildState(bucket.items) }))
}

export function sidebarSessionSections(
  sessions: Session[],
  projects: Project[],
  options: SidebarFilterOptions = {},
): SessionSections {
  const sortBy = options.sortBy ?? 'updated'
  const show = options.show ?? 'all'
  const groupBy = options.groupBy ?? 'project'
  const spaces = options.spaces ?? []
  const now = options.now ?? new Date()

  const visible = show === 'all' ? sessions : sessions.filter((session) => matchesShowFilter(session, show))
  const items = groupSessionsForDisplay(visible, sortBy)
  const pinnedItems = withLocalChildState(items.filter((item) => item.session.pinned))
  const recentItems = withLocalChildState(items.filter((item) => !item.session.pinned))

  if (groupBy === 'none') {
    return { pinnedItems, recentItems, groups: [], ungrouped: recentItems }
  }
  if (groupBy === 'date') {
    const groups = groupByBuckets(
      recentItems,
      [
        { key: 'date:today', label: 'Today' },
        { key: 'date:yesterday', label: 'Yesterday' },
        { key: 'date:7d', label: 'Previous 7 days' },
        { key: 'date:30d', label: 'Previous 30 days' },
        { key: 'date:older', label: 'Older' },
      ],
      (item) => dateBucket(sessionTime(item.session, sortBy), now),
    )
    return { pinnedItems, recentItems, groups, ungrouped: [] }
  }
  if (groupBy === 'status') {
    const groups = groupByBuckets(
      recentItems,
      [
        { key: 'status:running', label: 'Running' },
        { key: 'status:unread', label: 'Unread' },
        { key: 'status:attention', label: 'Needs attention' },
        { key: 'status:idle', label: 'Idle' },
      ],
      (item) => statusBucket(item.session),
    )
    return { pinnedItems, recentItems, groups, ungrouped: [] }
  }
  if (groupBy === 'environment') {
    const groups = groupByBuckets(
      recentItems,
      [
        { key: 'env:cloud', label: 'Cloud' },
        { key: 'env:local', label: 'Local' },
      ],
      (item) => (item.session.backend === 'cloud'
        ? { key: 'env:cloud', label: 'Cloud' }
        : { key: 'env:local', label: 'Local' }),
    )
    return { pinnedItems, recentItems, groups, ungrouped: [] }
  }
  return { pinnedItems, recentItems, ...sessionsBySavedProject(recentItems, projects, spaces, show === 'all') }
}

export function applySessionDragOrder(
  groups: SessionProjectGroup[],
  order: SessionBlockKey[] | null,
): SessionProjectGroup[] {
  if (!order) return groups
  const rank = new Map(order.map((key, index) => [key, index]))
  return [...groups].sort(
    (a, b) =>
      (rank.get(sessionProjectBlockKey(a.key)) ?? order.length) -
      (rank.get(sessionProjectBlockKey(b.key)) ?? order.length),
  )
}

export function applySidebarProjectOrder(projects: Project[], blockOrder: SessionBlockKey[]): Project[] {
  const byBlockKey = new Map<SessionBlockKey, Project>(
    projects.map((project) => [sessionProjectBlockKey(project.path), project]),
  )
  const reordered = blockOrder
    .map((key) => byBlockKey.get(key))
    .filter((project): project is Project => Boolean(project))
  const reorderedPaths = new Set(reordered.map((project) => project.path))
  return projects.map((project) => (reorderedPaths.has(project.path) ? reordered.shift()! : project))
}

export function sessionPage(items: SessionListItem[], limit: number) {
  return { items: items.slice(0, limit), total: items.length }
}

export function sessionDisplayBlocks(
  groups: SessionProjectGroup[],
  ungrouped: SessionListItem[],
  expandedProjects: Set<string>,
  showAllUngrouped: boolean,
  collapsedProjects: Set<string>,
): SessionDisplayBlock[] {
  const blocks: SessionDisplayBlock[] = groups.map((group) => ({
    kind: 'project',
    key: sessionProjectBlockKey(group.key),
    group,
    items: sessionPage(group.items, expandedProjects.has(group.key) ? group.items.length : PROJECT_SESSION_LIMIT).items,
    collapsed: collapsedProjects.has(group.key),
  }))
  if (!ungrouped.length) return blocks

  blocks.push({
    kind: 'ungrouped',
    key: UNGROUPED_BLOCK_KEY,
    ...sessionPage(ungrouped, showAllUngrouped ? ungrouped.length : PROJECT_SESSION_LIMIT),
  })
  return blocks
}
