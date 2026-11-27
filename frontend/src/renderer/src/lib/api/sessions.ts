import type { CloudThreadGitRepoSpec } from '@/lib/api/cloudThreadRepos'
import type { UsageWindow } from '@/lib/usageWindow'
import { ApiError, readAPIResponse } from '@/lib/api/response'
import { queryOptions } from '@tanstack/react-query'
import { cloudWorkspaceHeaders } from '@/lib/api/cloudScope'
import { request } from '@/lib/api/client'
import { sanitizeAssetOrigin, type ResolvedAsset } from '@/lib/artifacts'
import { backendTransportUrl } from '@/lib/clientRuntime'
import type { MessageContextInput } from '@/lib/messageContext'
import { telemetry } from '@/lib/telemetry'
import { keys } from '../query/keys'
import {
  apiBaseUrl,
  apiEmbeddedGetUrl,
  apiFetch,
  del,
  get,
  isCloudApiSession,
  post,
  put,
  resolveBackendUrl,
  secureBackendConnection,
} from './client'
import { updateThreadUserState } from './cloudSpaces'
import {
  cloudRunFileBytes,
  cloudRunFileDownloadUrl,
  cloudRunFileUrl,
  grantCloudRunAsset,
  normalizeAssetGrant,
  readCloudRunFile,
  type AssetGrantWire,
} from './cloudUploads'
import {
  cancelCloudThread,
  cloudThreadToSession,
  createCloudThread,
  deleteCloudThread,
  fetchCloudThreadMessages,
  getCloudThread,
  listCloudThreads,
  patchCloudThread,
  steerCloudThread,
} from './cloudThreads'
import {
  fileKey,
  type Attachment,
  type ChatMessage,
  type DailyUsage,
  type HealthResponse,
  type QueuedMessageInput,
  type RepoChanges,
  type RepoFileChange,
  type RepoFilePatch,
  type RepoInfo,
  type RepoRevision,
  type SessionFileRead,
  type Session,
  type SessionEvent,
  type SessionMessages,
  type SessionOverview,
} from './types'

export interface CreateSessionInput {
  title?: string
  runtime?: 'acp'
  agent?: string
  directory?: string
  worktree?: boolean
  artifact_surface?: 'chat' | 'canvas'
  // Per-session overrides of the Settings > Agents defaults; model_provider
  // applies to ACP agents with provider-backed models.
  model_provider?: string
  backend?: 'cloud'
  model_profile_id?: string
  cloud_credential_profile_id?: string
  cloud_workspace_id?: string
  model?: string
  reasoning_effort?: string
  config_options?: Record<string, string>
  space_id?: string
  visibility?: 'private' | 'space'
  machine_id?: string
  executor_target?: string
  git_repos?: CloudThreadGitRepoSpec[]
  git_repo?: string
  git_base_branch?: string
  git_branch?: string
}

export async function createSession(input: CreateSessionInput = {}): Promise<Session> {
  if (isCloudApiSession()) {
    if (input.cloud_credential_profile_id) throw new Error('Native runs must be started from the paired Desktop')
    const thread = await createCloudThread({
      title: input.title,
      space_id: input.space_id,
      model_profile_id: input.model_profile_id,
      model: input.model,
      reasoning_effort: input.reasoning_effort,
      executor_target: input.executor_target ?? input.machine_id,
      git_repos: input.git_repos,
      git_repo: input.git_repo,
      git_base_branch: input.git_base_branch,
      git_branch: input.git_branch,
    })
    return cloudThreadToSession(thread)
  }
  const { config_options, cloud_workspace_id, ...payload } = input
  const session = await request<Session>('/v1/sessions', {
    method: 'POST',
    headers: { ...cloudWorkspaceHeaders(cloud_workspace_id), 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
  if (session.backend !== 'cloud') {
    for (const [id, value] of Object.entries(config_options ?? {})) {
      await setSessionAgentConfig(session.id, id, value)
    }
  }
  telemetry.threadCreated({
    worktree: Boolean(input.worktree),
    hasDirectory: Boolean(input.directory),
    hasModelOverride: Boolean(input.model),
    hasProviderOverride: Boolean(input.model_provider),
    hasReasoningEffort: Boolean(input.reasoning_effort),
  })
  return session
}

export async function getSession(id: string, signal?: AbortSignal): Promise<Session> {
  if (isCloudApiSession()) {
    return cloudThreadToSession(await getCloudThread(id, signal))
  }
  return get<Session>(`/v1/sessions/${id}`, { signal })
}

export function setSessionAgentConfig(sessionId: string, id: string, value: string): Promise<void> {
  return put<void>(`/v1/sessions/${sessionId}/agent/config`, { id, value })
}

export const sessionQuery = (id: string) =>
  queryOptions({
    queryKey: keys.session(id),
    queryFn: () => getSession(id),
    staleTime: 30_000,
  })

// Live events a mounted thread receives over SSE (useSessionEvents writes
// them here); there is nothing to fetch, and the cache outlives the page.
export const sessionEventsQuery = (id: string) =>
  queryOptions<SessionEvent[]>({
    queryKey: keys.sessionEvents(id),
    queryFn: () => [],
    initialData: [],
    staleTime: Infinity,
    gcTime: Infinity,
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

// sessionFilePath addresses a session file's raw content: GET reads it with an
// ETag, and PUT replaces it, honouring If-Match.
export function sessionFilePath(sessionId: string, path: string): string {
  const params = new URLSearchParams({ path, raw: '1' })
  return `/v1/sessions/${encodeURIComponent(sessionId)}/file?${params.toString()}`
}

// saveSessionFile writes content over the version read and returns the
// version written; a file changed since that read is refused with 412.
export async function saveSessionFile(sessionId: string, path: string, content: string, version: string): Promise<string> {
  const response = await apiFetch(sessionFilePath(sessionId, path), { method: 'PUT', headers: { 'If-Match': version }, body: content })
  if (!response.ok) throw new ApiError(response.status, `could not save ${path}`)
  return response.headers.get('ETag')!
}

// createSessionFile writes a file that does not exist yet, with its folder.
export async function createSessionFile(sessionId: string, path: string, content: Blob): Promise<void> {
  const response = await apiFetch(sessionFilePath(sessionId, path), { method: 'PUT', headers: { 'If-None-Match': '*' }, body: content })
  if (!response.ok) throw new ApiError(response.status, `could not create ${path}`)
}

export function sessionCloudRunId(session?: Pick<Session, 'id' | 'backend' | 'run_id'>): string | undefined {
  if (!session) return undefined
  if (session.backend === 'cloud' || isCloudApiSession()) {
    return session.run_id || session.id
  }
  return undefined
}

export function effectiveCloudRunId(sessionId: string, cloudRunId?: string): string | undefined {
  if (cloudRunId) return cloudRunId
  return isCloudApiSession() ? sessionId : undefined
}

// sessionFileBytes reads a file with the connection's own credentials. A
// reader that fetches uses it; sessionFileRawUrl is for an element that loads
// its own address and so needs the short-lived ticket in it.
export async function sessionFileBytes(
  sessionId: string,
  path: string,
  signal?: AbortSignal,
  cloudRunId?: string,
): Promise<ArrayBuffer> {
  const runId = effectiveCloudRunId(sessionId, cloudRunId)
  if (runId) {
    return cloudRunFileBytes(runId, path, signal)
  }
  const response = await apiFetch(sessionFilePath(sessionId, path), { signal })
  if (!response.ok) await readAPIResponse(response)
  return response.arrayBuffer()
}

export function sessionFileRawUrl(sessionId: string, path: string, cloudRunId?: string): string {
  const runId = effectiveCloudRunId(sessionId, cloudRunId)
  if (runId) {
    return cloudRunFileUrl(runId, path)
  }
  return apiEmbeddedGetUrl(sessionFilePath(sessionId, path))
}

export function sessionFileDownloadUrl(sessionId: string, path: string, cloudRunId?: string): string {
  const runId = effectiveCloudRunId(sessionId, cloudRunId)
  if (runId) {
    return cloudRunFileDownloadUrl(runId, path)
  }
  return apiEmbeddedGetUrl(sessionFilePath(sessionId, path) + '&download=1')
}

export function activePluginAssetOrigin(cloudRunId?: string): string {
  if (cloudRunId) {
    const configured = import.meta.env?.VITE_JAZ_CLOUD_ASSET_ORIGIN?.trim()
    return sanitizeAssetOrigin(configured || 'https://storage.googleapis.com')
  }
  return sanitizeAssetOrigin(backendTransportUrl(`${apiBaseUrl()}/`))
}

export async function grantSessionAsset(
  sessionId: string,
  relpath: string,
  signal?: AbortSignal,
  cloudRunId?: string,
): Promise<ResolvedAsset> {
  const runId = effectiveCloudRunId(sessionId, cloudRunId)
  if (runId) {
    return grantCloudRunAsset(runId, relpath, signal)
  }
  const wire = await post<AssetGrantWire>(
    `/v1/sessions/${encodeURIComponent(sessionId)}/assets/grant`,
    { path: relpath },
    signal,
  )
  const rawUrl = wire.url?.trim() ?? ''
  const resolved = rawUrl.startsWith('/')
    ? resolveBackendUrl(rawUrl, apiBaseUrl(), 'grantSessionAsset')
    : rawUrl
  if (!resolved || !secureBackendConnection(resolved)) {
    throw new Error('Asset grant URL must use HTTPS or loopback')
  }
  return normalizeAssetGrant(wire, backendTransportUrl(resolved))
}

export function sessionAttachmentUrl(sessionId: string, attachmentId: string): string {
  return apiEmbeddedGetUrl(
    `/v1/sessions/${encodeURIComponent(sessionId)}/attachments/${encodeURIComponent(attachmentId)}`,
  )
}

export async function uploadSessionAttachment(sessionId: string, file: File, signal?: AbortSignal): Promise<Attachment> {
  const form = new FormData()
  // Electron's local protocol can fail to forward a File restored from a draft.
  form.append('file', file.slice(0, file.size, file.type), file.name)
  const res = await apiFetch(`/v1/sessions/${sessionId}/attachments`, {
    method: 'POST',
    body: form,
    signal,
  })
  return readAPIResponse<Attachment>(res)
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

export const dailyUsageQuery = (window: UsageWindow = { days: 30 }) => {
  const timezone = usageTimezone()
  return queryOptions({
    queryKey: keys.usageDaily(window, timezone),
    queryFn: async (): Promise<DailyUsage[]> => {
      const params = usageParams(window, timezone)
      const data = await get<{ days: DailyUsage[] | null }>(`/v1/usage/daily?${params}`)
      return data.days ?? []
    },
    staleTime: 60_000,
    refetchOnMount: 'always',
    refetchInterval: 300_000,
  })
}

function usageParams(window: UsageWindow, timezone: string): URLSearchParams {
  const params = new URLSearchParams('days' in window ? { days: String(window.days) } : window)
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

export function createFilesystemDir(parent: string, name: string): Promise<{ path: string }> {
  return post<{ path: string }>('/v1/filesystem/dirs', { parent, name })
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
): Promise<{ path: string; parent: string; git: boolean; revision?: RepoRevision; dirs: FilesystemDir[] }> {
  return get<{
    path: string
    parent?: string
    git?: boolean
    revision?: RepoRevision
    dirs: FilesystemDir[] | null
  }>(`/v1/filesystem/dirs?path=${encodeURIComponent(path)}`).then((data) => ({
    path: data.path,
    parent: data.parent ?? '',
    git: data.git ?? false,
    revision: data.revision,
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

// Polled while mounted — the branch and upstream change as agents work.
export const sessionRepoQuery = (id: string) =>
  queryOptions({
    queryKey: keys.sessionRepo(id),
    queryFn: () => get<RepoInfo>(`/v1/sessions/${id}/repo`),
    staleTime: 15_000,
    refetchInterval: 30_000,
  })

// What the session changed: per-file +/− line counts vs the session's diff
// base. Deliberately not polled — computing it walks the whole tree, so it
// refreshes via invalidation at turn boundaries and after repo actions, and
// runs only while a surface showing it is mounted.
export const sessionRepoChangesQuery = (id: string) =>
  queryOptions({
    queryKey: keys.sessionRepoChanges(id),
    queryFn: () => get<RepoChanges>(`/v1/sessions/${id}/repo/changes`),
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

export const sessionFileQuery = (id: string, path: string, cloudRunId?: string) => {
  const runId = effectiveCloudRunId(id, cloudRunId)
  return queryOptions({
    queryKey: runId ? [...keys.sessionFile(id, path), 'cloud', runId] : keys.sessionFile(id, path),
    queryFn: ({ signal }) => readSessionFile(id, path, signal, runId),
    staleTime: 15_000,
  })
}

export function readSessionFile(
  id: string,
  path: string,
  signal?: AbortSignal,
  cloudRunId?: string,
): Promise<SessionFileRead> {
  const runId = effectiveCloudRunId(id, cloudRunId)
  if (runId) {
    return readCloudRunFile(runId, path, signal)
  }
  return get<SessionFileRead>(`/v1/sessions/${id}/file?path=${encodeURIComponent(path)}`, { signal })
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

export async function setSessionArchived(id: string, archived: boolean): Promise<Session> {
  if (isCloudApiSession()) {
    await updateThreadUserState(id, { archived })
    const session = await getSession(id)
    return { ...session, archived }
  }
  return post<Session>(`/v1/sessions/${id}/${archived ? 'archive' : 'unarchive'}`)
}

export async function setSessionPinned(id: string, pinned: boolean): Promise<Session> {
  if (isCloudApiSession()) {
    await updateThreadUserState(id, { pinned })
    const session = await getSession(id)
    return { ...session, pinned }
  }
  return post<Session>(`/v1/sessions/${id}/${pinned ? 'pin' : 'unpin'}`)
}

export async function setSessionTitle(id: string, title: string, isCloud = false): Promise<Session> {
  if (isCloud || isCloudApiSession()) {
    return cloudThreadToSession(await patchCloudThread(id, { title }))
  }
  return post<Session>(`/v1/sessions/${id}/rename`, { title })
}

export async function moveSessionToSpace(id: string, spaceId: string): Promise<Session> {
  return cloudThreadToSession(await patchCloudThread(id, { space_id: spaceId }))
}

export async function deleteSession(id: string): Promise<{ deleted: boolean; id: string }> {
  return deleteCloudThread(id)
}

// Stops the in-flight turn server-side (turns survive closed streams).
export function cancelSession(id: string, isCloud = false): Promise<{ ok: boolean }> {
  if (isCloud || isCloudApiSession()) {
    return cancelCloudThread(id)
  }
  return post<{ ok: boolean }>(`/v1/sessions/${id}/cancel`)
}

export function clearSessionGoal(id: string): Promise<void> {
  return del<void>(`/v1/sessions/${id}/goal`)
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

export async function mutateSessionQueue(id: string, mutation: QueueMutation, isCloud = false): Promise<Session> {
  if (isCloud || isCloudApiSession()) {
    if (mutation.op === 'append') {
      await steerCloudThread(id, mutation.message.text, 'queue')
      return getSession(id)
    }
    throw new Error('Cloud threads only support queueing new messages while running')
  }
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

export const sidebarSessionsQuery = queryOptions({
  queryKey: keys.sidebarSessions,
  queryFn: async ({ signal }) => {
    if (isCloudApiSession()) {
      return (await listCloudThreads(undefined, signal))
        .map(cloudThreadToSession)
        .filter((session) => !session.archived)
    }
    const { sessions } = await get<{ sessions: Session[] | null }>('/v1/sessions?include_children=true', { signal })
    return sessions ?? []
  },
  // Tighten the poll while a thread is running so status dots stay live.
  refetchInterval: (query) =>
    query.state.data?.some((session) => session.status === 'running') ? 3_000 : 15_000,
})

// Stored events carry only the acp session id and slug; session-constant
// labels arrive once per response in acp_meta. Fold them back onto events here
// so the rest of the app keeps the single contract: labels live on event.acp.
type SessionMessagesResponse = Omit<SessionMessages, 'messages' | 'events'> & {
  messages: ChatMessage[] | null
  events: SessionEvent[] | null
}

function hydrateEventLabels(data: SessionMessagesResponse): SessionEvent[] {
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

function normalizedSessionMessages(data: SessionMessagesResponse): SessionMessages {
  return {
    ...data,
    messages: data.messages ?? [],
    events: hydrateEventLabels(data),
  }
}

type SessionMessagesPageRequest = {
  beforeMessageSeq?: number
  beforeEventSeq?: number
  historyRevision?: number
  turns?: number
}

export async function getSessionMessagesPage(
  id: string,
  page: SessionMessagesPageRequest = {},
  signal?: AbortSignal,
): Promise<SessionMessages> {
  if (isCloudApiSession()) {
    return fetchCloudThreadMessages(id, signal)
  }
  const params = new URLSearchParams()
  if (page.beforeMessageSeq) params.set('before_message_seq', String(page.beforeMessageSeq))
  if (page.beforeEventSeq) params.set('before_event_seq', String(page.beforeEventSeq))
  if (page.historyRevision) params.set('history_revision', String(page.historyRevision))
  if (page.turns) params.set('turns', String(page.turns))
  const query = params.size ? `?${params.toString()}` : ''
  return normalizedSessionMessages(
    await get<SessionMessagesResponse>(`/v1/sessions/${id}/messages${query}`, { signal }),
  )
}

export const sessionMessagesQuery = (id: string) =>
  queryOptions({
    queryKey: keys.sessionMessages(id),
    queryFn: ({ signal }) => getSessionMessagesPage(id, {}, signal),
  })

export const sessionOverviewQuery = (id: string) =>
  queryOptions({
    queryKey: keys.sessionOverview(id),
    queryFn: ({ signal }) => get<SessionOverview>(`/v1/sessions/${id}/overview`, { signal }),
    refetchInterval: (query) => query.state.data?.plugin_views?.some((view) => view.reference) ? 5000 : false,
  })

export const healthQuery = queryOptions({
  queryKey: keys.health,
  queryFn: () => get<HealthResponse>('/health'),
  retry: false,
  refetchInterval: (query) => (query.state.status === 'error' ? 3_000 : 30_000),
})
