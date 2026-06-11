import { queryOptions } from '@tanstack/react-query'
import { keys } from '../query/keys'
import { apiBaseUrl, ApiError, get, post } from './client'
import type { Attachment, RepoInfo, Session, SessionMessages } from './types'

export function createSession(
  input: {
    title?: string
    runtime?: 'native' | 'acp'
    agent?: string
    directory?: string
    worktree?: boolean
    // Per-session overrides of the Settings > Agents defaults; model_provider
    // only applies to native sessions.
    model_provider?: string
    model?: string
    reasoning_effort?: string
  } = {},
): Promise<Session> {
  return post<Session>('/v1/sessions', input)
}

export async function uploadSessionAttachment(sessionId: string, file: File, signal?: AbortSignal): Promise<Attachment> {
  const form = new FormData()
  form.append('file', file)
  const res = await fetch(`${apiBaseUrl()}/v1/sessions/${sessionId}/attachments`, {
    method: 'POST',
    body: form,
    signal,
  })
  if (!res.ok) {
    let message = `${res.status} ${res.statusText}`
    try {
      const body = (await res.json()) as { error?: string }
      if (body.error) message = body.error
    } catch {
      // keep status text
    }
    throw new ApiError(res.status, message)
  }
  return (await res.json()) as Attachment
}

// Configured ACP agents the new-thread page can offer as a runtime. Resilient
// by design: any failure (older backend without the route, no agents) yields an
// empty list so the runtime selector simply doesn't appear.
export const acpAgentsQuery = queryOptions({
  queryKey: keys.acpAgents,
  queryFn: async () => {
    try {
      const data = await get<{ agents: string[] | null }>('/v1/acp/agents')
      return data.agents ?? []
    } catch {
      return []
    }
  },
})

export interface WorkspaceDir {
  name: string
  git: boolean
}

// Lists immediate subdirectories of a workspace-relative path so the directory
// picker can browse where an ACP session runs ('' is the workspace root). `git`
// flags whether the browsed path (and each entry) is a git repository root.
export function listWorkspaceDirs(
  path: string,
): Promise<{ path: string; git: boolean; dirs: WorkspaceDir[] }> {
  return get<{ path: string; git?: boolean; dirs: WorkspaceDir[] | null }>(
    `/v1/workspace/dirs?path=${encodeURIComponent(path)}`,
  ).then((data) => ({ path: data.path, git: data.git ?? false, dirs: data.dirs ?? [] }))
}

export interface WorkspaceFileEntry {
  path: string
  dir: boolean
}

export interface WorkspaceFileIndex {
  root: string
  entries: WorkspaceFileEntry[]
  truncated: boolean
}

// Shallow file/dir index of a workspace directory for the composer's
// @-mention picker. `root` echoes the server-resolved absolute directory so
// tagged entries can expand to full paths. Resilient: any failure (older
// backend without the route) yields an empty index so @ is simply inert. The
// short staleTime avoids per-keystroke refetches while the menu is open but
// still picks up files the agent just created on the next mention.
export const workspaceFilesQuery = (path: string) =>
  queryOptions({
    queryKey: keys.workspaceFiles(path),
    queryFn: async (): Promise<WorkspaceFileIndex> => {
      try {
        const data = await get<{
          root: string
          entries: WorkspaceFileEntry[] | null
          truncated?: boolean
        }>(`/v1/workspace/files?path=${encodeURIComponent(path)}`)
        return { root: data.root, entries: data.entries ?? [], truncated: data.truncated ?? false }
      } catch {
        return { root: '', entries: [], truncated: false }
      }
    },
    staleTime: 30_000,
  })

// Git/forge state of the session's working directory. Resilient: any failure
// (older backend without the route) reads as "not a repo" so the titlebar repo
// button simply doesn't render. Polled while mounted — the branch and upstream
// change as agents work.
export const sessionRepoQuery = (id: string) =>
  queryOptions({
    queryKey: keys.sessionRepo(id),
    queryFn: async (): Promise<RepoInfo> => {
      try {
        return await get<RepoInfo>(`/v1/sessions/${id}/repo`)
      } catch {
        return { git: false }
      }
    },
    staleTime: 15_000,
    refetchInterval: 30_000,
  })

// Publishes the session's current branch to its remote (git push -u) and
// returns the refreshed repo state; Create PR calls this first when the
// branch has no upstream yet.
export function pushSessionRepoBranch(id: string): Promise<RepoInfo> {
  return post<RepoInfo>(`/v1/sessions/${id}/repo/push`)
}

// Stages and commits everything in the session's working directory; the
// backend defaults the message to the session title.
export function commitSessionRepo(id: string, message?: string): Promise<RepoInfo> {
  return post<RepoInfo>(`/v1/sessions/${id}/repo/commit`, message ? { message } : {})
}

// Commits the worktree's outstanding work on its branch and merges that
// branch into the repo's main checkout. `moved` reports whether the session's
// cwd followed — true for native sessions, false for ACP agents bound to
// their spawn cwd. Conflicting merges are aborted server-side and surface as
// errors; the main checkout is never left mid-merge.
export function mergeSessionRepo(
  id: string,
): Promise<{ cwd: string; moved: boolean; info: RepoInfo }> {
  return post<{ cwd: string; moved: boolean; info: RepoInfo }>(`/v1/sessions/${id}/repo/merge`)
}

export function setSessionArchived(id: string, archived: boolean): Promise<Session> {
  return post<Session>(`/v1/sessions/${id}/${archived ? 'archive' : 'unarchive'}`)
}

// Stops the in-flight turn server-side (turns survive closed streams).
export function cancelSession(id: string): Promise<{ ok: boolean }> {
  return post<{ ok: boolean }>(`/v1/sessions/${id}/cancel`)
}

export type QueueMutation =
  | { op: 'append'; text: string }
  | { op: 'delete'; index: number; expected?: string }
  | { op: 'edit'; index: number; text: string; expected?: string }
  | { op: 'move'; from: number; to: number; expected?: string }
  | { op: 'steer'; index: number; expected?: string }
  | { op: 'replace'; messages: string[] }

export function mutateSessionQueue(id: string, mutation: QueueMutation): Promise<Session> {
  return post<Session>(`/v1/sessions/${id}/queue`, mutation)
}

export function answerSessionInteractiveResponse(
  id: string,
  input: {
    request_id?: string
    option_id?: string
    text?: string
    plan_requested?: boolean
    parent_visible?: boolean
    answers?: Record<string, { answers: string[] }>
  },
): Promise<{ ok: boolean }> {
  return post<{ ok: boolean }>(`/v1/sessions/${id}/interactive-response`, input)
}

export const archivedSessionsQuery = queryOptions({
  queryKey: keys.archivedSessions,
  queryFn: async () => {
    const data = await get<{ sessions: Session[] | null }>(
      '/v1/sessions?archived=true&include_children=true',
    )
    return groupSessionsForDisplay(data.sessions ?? [])
  },
})

export const SIDEBAR_SESSION_LIMIT = 7

function sessionTime(session: Session): number {
  const ms = Date.parse(session.updated_at)
  return Number.isNaN(ms) ? 0 : ms
}

function compareSessions(a: Session, b: Session): number {
  return sessionTime(b) - sessionTime(a) || a.id.localeCompare(b.id)
}

// A display row: `child` marks a session whose parent renders directly above
// it in the same list; rows draw a branch connector for those. Orphans (and
// archived children whose parent isn't in the list) render as roots.
export interface SessionListItem {
  session: Session
  child: boolean
}

export function groupSessionsForDisplay(sessions: Session[]): SessionListItem[] {
  const byID = new Map(sessions.map((session) => [session.id, session]))
  const children = new Map<string, Session[]>()
  const roots: Session[] = []

  for (const session of sessions) {
    if (session.parent_id && session.parent_id !== session.id && byID.has(session.parent_id)) {
      children.set(session.parent_id, [...(children.get(session.parent_id) ?? []), session])
    } else {
      roots.push(session)
    }
  }

  const groupTimes = new Map<string, number>()
  const groupTime = (session: Session, visiting = new Set<string>()): number => {
    const cached = groupTimes.get(session.id)
    if (cached !== undefined) return cached
    if (visiting.has(session.id)) return sessionTime(session)

    visiting.add(session.id)
    let latest = sessionTime(session)
    for (const child of children.get(session.id) ?? []) {
      latest = Math.max(latest, groupTime(child, visiting))
    }
    visiting.delete(session.id)
    groupTimes.set(session.id, latest)
    return latest
  }

  const compareGroups = (a: Session, b: Session): number =>
    groupTime(b) - groupTime(a) || compareSessions(a, b)

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

export const sidebarSessionsQuery = queryOptions({
  queryKey: keys.sidebarSessions,
  queryFn: async () => {
    const data = await get<{ sessions: Session[] | null }>('/v1/sessions?include_children=true')
    return groupSessionsForDisplay(data.sessions ?? [])
  },
  // Tighten the poll while a thread is running so status dots stay live.
  refetchInterval: (query) =>
    query.state.data?.some((item) => item.session.status === 'running') ? 3_000 : 15_000,
})

export const allSessionsQuery = queryOptions({
  queryKey: keys.allSessions,
  queryFn: async () => {
    const data = await get<{ sessions: Session[] | null }>('/v1/sessions?include_children=true')
    return groupSessionsForDisplay(data.sessions ?? [])
  },
})

export const sessionMessagesQuery = (id: string) =>
  queryOptions({
    queryKey: keys.sessionMessages(id),
    queryFn: async () => {
      // Go marshals empty slices as null; normalize once here.
      const data = await get<SessionMessages>(`/v1/sessions/${id}/messages`)
      return {
        ...data,
        messages: data.messages ?? [],
        activity: data.activity ?? [],
        events: data.events ?? [],
      }
    },
  })

export const healthQuery = queryOptions({
  queryKey: keys.health,
  queryFn: () => get<{ ok: boolean }>('/health'),
  retry: false,
  refetchInterval: (query) => (query.state.status === 'error' ? 3_000 : 30_000),
})
