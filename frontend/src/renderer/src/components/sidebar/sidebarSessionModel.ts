import type { Project, SessionListItem } from '@/lib/api/sessions'

export const SESSION_PAGE_SIZE = 50
const PROJECT_SESSION_LIMIT = 5
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
  const groups = new Map<string, SessionProjectGroup>(projects.map((project) => [
    project.path,
    { key: project.path, label: project.name, items: [] },
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
    const group = groups.get(sidebarProjectPath(item, byID, childProjectByParentID))
    if (!group) {
      ungrouped.push(item)
      continue
    }
    group.items.push(item)
  }

  return {
    groups: [...groups.values()]
      .filter((group) => group.items.length > 0)
      .map((group) => ({ ...group, items: withLocalChildState(group.items) })),
    ungrouped: withLocalChildState(ungrouped),
  }
}

export function sidebarSessionSections(items: SessionListItem[], projects: Project[]): SessionSections {
  const pinnedItems = withLocalChildState(items.filter((item) => item.session.pinned))
  const recentItems = withLocalChildState(items.filter((item) => !item.session.pinned))
  return { pinnedItems, recentItems, ...sessionsBySavedProject(recentItems, projects) }
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
