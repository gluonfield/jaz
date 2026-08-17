import type { Project, SessionListItem } from '@/lib/api/sessions'

export const SESSION_PAGE_SIZE = 50
const UNGROUPED_BLOCK_KEY = 'ungrouped'
export type SessionBlockKey = `project:${string}` | typeof UNGROUPED_BLOCK_KEY

export type SessionProjectGroup = {
  key: string
  label: string
  items: SessionListItem[]
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

function sessionsBySavedProject(items: SessionListItem[], projects: Project[]) {
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
    const group = groups.get(project.path) ?? { key: project.path, label: project.name, items: [] }
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

export function sidebarSessionSections(items: SessionListItem[], projects: Project[]): SessionSections {
  const pinnedItems = withLocalChildState(items.filter((item) => item.session.pinned))
  const projectItems = withLocalChildState(items.filter((item) => !item.session.pinned))
  const recentItems = [...projectItems].sort((a, b) => sessionListItemTime(b) - sessionListItemTime(a))
  return { pinnedItems, recentItems, ...sessionsBySavedProject(projectItems, projects) }
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

function sessionListItemTime(item: SessionListItem): number {
  const ms = Date.parse(item.session.last_attention_at || item.session.updated_at)
  return Number.isNaN(ms) ? 0 : ms
}

function sessionListItemsTime(items: SessionListItem[]): number {
  return Math.max(0, ...items.map(sessionListItemTime))
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
    items: sessionPage(group.items, expandedProjects.has(group.key) ? group.items.length : SESSION_PAGE_SIZE).items,
    collapsed: collapsedProjects.has(group.key),
  }))
  if (!ungrouped.length) return blocks

  const ungroupedBlock: SessionDisplayBlock = {
    kind: 'ungrouped',
    key: UNGROUPED_BLOCK_KEY,
    ...sessionPage(ungrouped, showAllUngrouped ? ungrouped.length : SESSION_PAGE_SIZE),
  }
  const time = sessionListItemsTime(ungrouped)
  const index = groups.findIndex((group) => time > sessionListItemsTime(group.items))
  if (index === -1) blocks.push(ungroupedBlock)
  else blocks.splice(index, 0, ungroupedBlock)
  return blocks
}
