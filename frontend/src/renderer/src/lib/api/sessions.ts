import { queryOptions } from '@tanstack/react-query'
import type { MessageContextInput } from '@/lib/messageContext'
import { telemetry } from '@/lib/telemetry'
import { keys } from '../query/keys'
import { apiEmbeddedGetUrl, apiFetch, ApiError, del, get, post, put } from './client'
import {
  fileKey,
  type Attachment,
  type DailyUsage,
  type HealthResponse,
  type QueuedMessageInput,
  type RepoChanges,
  type RepoFileChange,
  type RepoFilePatch,
  type RepoInfo,
  type SessionFileRead,
  type Session,
  type SessionEvent,
  type SessionMessages,
} from './types'

export interface CreateSessionInput {
  title?: string
  runtime?: 'acp'
  agent?: string
  directory?: string
  worktree?: boolean
  // Per-session overrides of the Settings > Agents defaults; model_provider
  // applies to ACP agents with provider-backed models.
  model_provider?: string
  model?: string
  reasoning_effort?: string
}

export async function createSession(input: CreateSessionInput = {}): Promise<Session> {
  const session = await post<Session>('/v1/sessions', input)
  telemetry.threadCreated({
    worktree: Boolean(input.worktree),
    hasDirectory: Boolean(input.directory),
    hasModelOverride: Boolean(input.model),
    hasProviderOverride: Boolean(input.model_provider),
    hasReasoningEffort: Boolean(input.reasoning_effort),
  })
  return session
}

export function getSession(id: string): Promise<Session> {
  return get<Session>(`/v1/sessions/${id}`)
}

export const sessionQuery = (id: string) =>
  queryOptions({
    queryKey: keys.session(id),
    queryFn: () => getSession(id),
    staleTime: 30_000,
  })

export interface SideChatMessageInput {
  id: string
  message: string
  contexts?: MessageContextInput[]
  attachment_ids?: string[]
}

export function sendSessionSideChat(id: string, input: SideChatMessageInput): Promise<{ ok: boolean }> {
  return post<{ ok: boolean }>(`/v1/sessions/${id}/side-chat`, input)
}

export function sessionFileRawUrl(sessionId: string, path: string): string {
  const params = new URLSearchParams({ path, raw: '1' })
  return apiEmbeddedGetUrl(`/v1/sessions/${encodeURIComponent(sessionId)}/file?${params.toString()}`)
}

export async function uploadSessionAttachment(sessionId: string, file: File, signal?: AbortSignal): Promise<Attachment> {
  const form = new FormData()
  form.append('file', file)
  const res = await apiFetch(`/v1/sessions/${sessionId}/attachments`, {
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

export const dailyUsageQuery = (days = 30) => {
  const timezone = usageTimezone()
  return queryOptions({
    queryKey: keys.usageDaily(days, timezone),
    queryFn: async (): Promise<DailyUsage[]> => {
      const params = usageParams(days, timezone)
      const data = await get<{ days: DailyUsage[] | null }>(`/v1/usage/daily?${params}`)
      return data.days ?? []
    },
    staleTime: 60_000,
    refetchOnMount: 'always',
    refetchInterval: 300_000,
  })
}

function usageParams(days: number, timezone: string): URLSearchParams {
  const params = new URLSearchParams({ days: String(days) })
  if (timezone) params.set('timezone', timezone)
  else params.set('tz_offset_minutes', String(new Date().getTimezoneOffset()))
  return params
}

function usageTimezone(): string {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || ''
  } catch {
    return ''
  }
}

export interface Project {
  name: string
  path: string
  git: boolean
}

export interface FilesystemDir {
  name: string
  path: string
  git: boolean
}

export const projectsQuery = queryOptions({
  queryKey: keys.projects,
  queryFn: async () => {
    const data = await get<{ projects: Project[] | null }>('/v1/projects')
    return data.projects ?? []
  },
})

export function addProject(path: string): Promise<Project> {
  return post<Project>('/v1/projects', { path })
}

export function deleteProject(path: string): Promise<Project[]> {
  return del<{ projects: Project[] | null }>(`/v1/projects?path=${encodeURIComponent(path)}`).then(
    (data) => data.projects ?? [],
  )
}

export function reorderProjects(paths: string[]): Promise<Project[]> {
  return put<{ projects: Project[] | null }>('/v1/projects/order', { paths }).then((data) => data.projects ?? [])
}

export function listFilesystemDirs(
  path: string,
): Promise<{ path: string; parent: string; git: boolean; dirs: FilesystemDir[] }> {
  return get<{
    path: string
    parent?: string
    git?: boolean
    dirs: FilesystemDir[] | null
  }>(`/v1/filesystem/dirs?path=${encodeURIComponent(path)}`).then((data) => ({
    path: data.path,
    parent: data.parent ?? '',
    git: data.git ?? false,
    dirs: data.dirs ?? [],
  }))
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

// Shallow file/dir index of a session directory for the composer's @-mention
// picker. `root` echoes the server-resolved absolute directory so tagged
// entries can expand to full paths. Resilient: any failure (older backend
// without the route) yields an empty index so @ is simply inert. The short
// staleTime avoids per-keystroke refetches while the menu is open but still
// picks up files the agent just created on the next mention.
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
// (older backend without the route) reads as "not a repo" so repo actions
// simply don't render. Polled while mounted — the branch and upstream change
// as agents work.
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

// What the session changed: per-file +/− line counts vs the session's diff
// base. Deliberately not polled — computing it walks the whole tree, so it
// refreshes via invalidation at turn boundaries and after repo actions, and
// runs only while a surface showing it is mounted. Failures (older backend)
// read as "no changes" so the section simply doesn't render.
export const sessionRepoChangesQuery = (id: string) =>
  queryOptions({
    queryKey: keys.sessionRepoChanges(id),
    queryFn: async (): Promise<RepoChanges> => {
      try {
        return await get<RepoChanges>(`/v1/sessions/${id}/repo/changes`)
      } catch {
        return { files: [], total_added: 0, total_deleted: 0 }
      }
    },
    staleTime: 30_000,
  })

// One file's unified diff, fetched only when the user opens that file in the
// Code Diff panel. The request carries the summary row's identity — status,
// rename source, resolved base — so the patch matches the row even if the
// tree moves between fetches. Cached per fileKey; the sessionRepo prefix
// invalidation marks it stale alongside the summary.
export const sessionRepoFileDiffQuery = (id: string, file: RepoFileChange, base?: string) =>
  queryOptions({
    queryKey: keys.sessionRepoDiff(id, fileKey(file), base ?? '', file.old_path ?? ''),
    queryFn: () => {
      const params = new URLSearchParams({ path: file.path })
      if (file.old_path) params.set('old_path', file.old_path)
      if (file.status === 'untracked') params.set('untracked', '1')
      if (base) params.set('base', base)
      return get<RepoFilePatch>(`/v1/sessions/${id}/repo/diff?${params}`)
    },
    staleTime: 30_000,
  })

export const sessionFileQuery = (id: string, path: string) =>
  queryOptions({
    queryKey: keys.sessionFile(id, path),
    queryFn: () => readSessionFile(id, path),
    staleTime: 15_000,
  })

export function readSessionFile(id: string, path: string): Promise<SessionFileRead> {
  return get<SessionFileRead>(`/v1/sessions/${id}/file?path=${encodeURIComponent(path)}`)
}

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
// cwd followed — false for ACP agents bound to
// their spawn cwd. Conflicting merges are aborted server-side and surface as
// errors; the main checkout is never left mid-merge.
export function mergeSessionRepo(
  id: string,
): Promise<{ cwd: string; moved: boolean; info: RepoInfo }> {
  return post<{ cwd: string; moved: boolean; info: RepoInfo }>(`/v1/sessions/${id}/repo/merge`)
}

export function mergeFromMainSessionRepo(id: string): Promise<RepoInfo> {
  return post<RepoInfo>(`/v1/sessions/${id}/repo/merge-from-main`)
}

export function restoreSessionWorktree(id: string): Promise<RepoInfo> {
  return post<RepoInfo>(`/v1/sessions/${id}/repo/restore-worktree`)
}

export function setSessionArchived(id: string, archived: boolean): Promise<Session> {
  return post<Session>(`/v1/sessions/${id}/${archived ? 'archive' : 'unarchive'}`)
}

export function setSessionPinned(id: string, pinned: boolean): Promise<Session> {
  return post<Session>(`/v1/sessions/${id}/${pinned ? 'pin' : 'unpin'}`)
}

export function setSessionTitle(id: string, title: string): Promise<Session> {
  return post<Session>(`/v1/sessions/${id}/rename`, { title })
}

// Stops the in-flight turn server-side (turns survive closed streams).
export function cancelSession(id: string): Promise<{ ok: boolean }> {
  return post<{ ok: boolean }>(`/v1/sessions/${id}/cancel`)
}

export function compactSession(id: string): Promise<{ ok: boolean; acp_state?: string }> {
  return post<{ ok: boolean; acp_state?: string }>(`/v1/sessions/${id}/compact`)
}

export type QueueMutation =
  | { op: 'append'; message: QueuedMessageInput }
  | { op: 'delete'; id: string }
  | { op: 'edit'; id: string; message: { text: string } }
  | { op: 'reorder'; ids: string[] }
  | { op: 'steer'; id: string }

export async function mutateSessionQueue(id: string, mutation: QueueMutation): Promise<Session> {
  const session = await post<Session>(`/v1/sessions/${id}/queue`, mutation)
  if (mutation.op === 'append') {
    telemetry.messageSent({
      queued: true,
      planRequested: Boolean(mutation.message.plan_requested),
      goalRequested: Boolean(mutation.message.goal_requested),
      attachmentCount: mutation.message.attachment_ids?.length ?? 0,
    })
  }
  return session
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
  const ms = Date.parse(session.last_attention_at || session.updated_at)
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

// Stored events carry only the acp session id and slug; session-constant
// labels arrive once per response in acp_meta. Fold them back onto events here
// so the rest of the app keeps the single contract: labels live on event.acp.
function hydrateEventLabels(data: SessionMessages): SessionEvent[] {
  return (data.events ?? []).map((event) => {
    const named = event.acp ? data.acp_meta?.[event.acp.id] : undefined
    if (!named) return event
    return {
      ...event,
      acp: {
        ...event.acp!,
        title: event.acp!.title || named.title,
        slug: event.acp!.slug || named.slug || '',
        model_provider: event.acp!.model_provider || named.model_provider,
        model: event.acp!.model || named.model,
        reasoning_effort: event.acp!.reasoning_effort || named.reasoning_effort,
      },
    }
  })
}

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
        events: hydrateEventLabels(data),
      }
    },
  })

export const healthQuery = queryOptions({
  queryKey: keys.health,
  queryFn: () => get<HealthResponse>('/health'),
  retry: false,
  refetchInterval: (query) => (query.state.status === 'error' ? 3_000 : 30_000),
})
