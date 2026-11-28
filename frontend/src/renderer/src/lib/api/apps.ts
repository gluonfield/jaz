import { queryOptions } from '@tanstack/react-query'
import { keys } from '../query/keys'
import { apiFetch, apiUrl, del, get, patch, post, put } from './client'
import { backendFetch } from '../clientRuntime'

export type AppVisibility = 'private' | 'selected' | 'space' | 'team'
// The kinds published as a bundle. A board has none: it holds other apps as tiles.
export type BundleKind = 'artifact' | 'document'
export type AppKind = BundleKind | 'board'

export interface AppIconSpec {
  kind?: string
  value?: string
  color?: string
  background?: string
}

export interface AppMember {
  user_id: string
  email?: string
  display_name?: string
  role: 'viewer' | 'editor'
}

export interface AppMemberInput {
  user_id: string
  role: 'viewer' | 'editor'
}

export interface AppBundleFile {
  path: string
  size_bytes: number
  sha256: string
  content_type: string
}

export interface AppExternalURL {
  url: string
  host: string
  allowlisted: boolean
  source_file?: string
}

export interface JazApp {
  id: string
  org_id?: string
  owner_user_id: string
  owner_display_name?: string
  owner_email?: string
  space_id?: string | null
  source_thread_id?: string
  // The loop that publishes this app onto its boards.
  loop_id?: string
  title: string
  description?: string
  icon?: AppIconSpec
  kind: AppKind
  visibility: AppVisibility
  current_version: number
  status: 'active' | 'deleting'
  bundle_sha256?: string
  entry_path?: string
  capabilities?: string[]
  allowed_tools?: string[]
  is_owner?: boolean
  can_edit?: boolean
  members?: AppMember[]
  created_at: string
  updated_at: string
}

export type BundledApp = JazApp & { kind: BundleKind }

export const hasBundle = (app: JazApp): app is BundledApp => app.kind !== 'board'

export interface AppVersion {
  app_id: string
  version: number
  bundle_sha256: string
  entry_path: string
  size_bytes: number
  file_count: number
  manifest?: Record<string, unknown>
  capabilities: string[]
  allowed_tools: string[]
  external_urls: AppExternalURL[]
  files?: AppBundleFile[]
  created_by?: string
  created_at: string
}

// A staged draft as saving it needs it.
export interface PendingAppDraft {
  draft_id: string
  bundle_sha256: string
  target_app_id?: string
  title: string
  kind: BundleKind
  capabilities: string[]
  allowed_tools: string[]
  expires_at: string
}

// Capabilities the user approves before an app is saved: anything beyond
// sandboxed storage, such as viewer AI credits, MCP tools or desktop privileges.
export function elevatedCapabilities(capabilities: string[]): string[] {
  return capabilities.filter((cap) => !cap.startsWith('storage:'))
}

export interface AppTicketResponse {
  ticket: string
  expires_at: string
  app_id: string
  version: number
  bundle_sha256: string
  entry_path: string
  content_origin: string
  content_url: string
}

export interface AppKVEntry {
  app_id?: string
  scope: 'personal' | 'shared'
  key: string
  value?: string
  revision: number
  size_bytes: number
  updated_by?: string
  updated_at: string
}

export interface AppKVListResponse {
  scope: 'personal' | 'shared'
  items: AppKVEntry[]
  total_bytes: number
  quota_bytes: number
}

export interface AppAICompleteResult {
  text: string
  model: string
  model_profile_id?: string
  stop_reason: string
  billed_user_id: string
  daily_calls_used?: number
  daily_calls_limit?: number
  usage?: {
    input_tokens: number
    cached_input_tokens: number
    output_tokens: number
    cost_usd?: number
  }
}

export interface OrgAppPolicy {
  org_id: string
  allow_shared_kv: boolean
  allow_ai_bridge: boolean
  allow_tool_bridge: boolean
  allow_desktop_full_access: boolean
  max_ai_calls_per_user_day: number
  max_kv_bytes_per_app: number
  updated_by?: string
  updated_at?: string
}

export interface AppRemixResult {
  thread: { id: string; title?: string }
  run_id: string
  source_app_id: string
  source_version: number
  bundle_sha256: string
  hydrated_files: string[]
}

export interface AppListFilter {
  scope?: 'all' | 'mine' | 'space' | 'team'
  spaceId?: string
  sourceThreadId?: string
  kind?: AppKind
  q?: string
}

export function appsQuery(filter?: AppListFilter) {
  return queryOptions({
    queryKey: keys.apps(filter),
    queryFn: async ({ signal }) => {
      const params = new URLSearchParams()
      if (filter?.scope) params.set('scope', filter.scope)
      if (filter?.spaceId) params.set('space_id', filter.spaceId)
      if (filter?.sourceThreadId) params.set('source_thread_id', filter.sourceThreadId)
      if (filter?.kind) params.set('kind', filter.kind)
      if (filter?.q?.trim()) params.set('q', filter.q.trim())
      const qs = params.toString()
      const data = await get<{ apps: JazApp[] | null }>(`/v1/apps${qs ? `?${qs}` : ''}`, { signal })
      const apps = data.apps ?? []
      // Older app servers may return the owner's full list for a thread filter.
      return filter?.sourceThreadId ? apps.filter((app) => app.status === 'active' && app.source_thread_id === filter.sourceThreadId) : apps
    },
  })
}

// A caller that learns of new versions elsewhere passes the version it knows,
// so a newer one fetches the detail again.
export function appDetailQuery(id: string, version?: number) {
  return queryOptions({
    queryKey: version === undefined ? keys.appDetail(id) : [...keys.appDetail(id), version],
    queryFn: async () =>
      get<{ app: JazApp; current_version: AppVersion | null }>(`/v1/apps/${encodeURIComponent(id)}`),
  })
}

// The app of a detail response, with the bundle hash its current version carries.
export function detailApp(detail: { app: JazApp; current_version: AppVersion | null }): JazApp {
  return { ...detail.app, bundle_sha256: detail.app.bundle_sha256 ?? detail.current_version?.bundle_sha256 }
}

// The entry file of an app's current version, as its library thumbnail draws it.
export function appPreviewQuery(app: Pick<JazApp, 'id' | 'current_version'>) {
  return queryOptions({
    queryKey: keys.appPreview(app.id, app.current_version),
    queryFn: async ({ signal }) => {
      const ticket = await issueAppTicket(app.id, app.current_version)
      return { entryPath: ticket.entry_path, content: await fetchAppContentText(ticket, ticket.entry_path, signal) }
    },
    staleTime: Infinity,
    retry: false,
  })
}

export function appVersionsQuery(id: string) {
  return queryOptions({
    queryKey: keys.appVersions(id),
    queryFn: async () => {
      const data = await get<{ versions: AppVersion[] | null }>(`/v1/apps/${encodeURIComponent(id)}/versions`)
      return data.versions ?? []
    },
  })
}

export const orgAppPolicyQuery = queryOptions({
  queryKey: keys.orgAppPolicy,
  queryFn: () => get<OrgAppPolicy>('/v1/org/app-policies'),
})

export interface AppBundleInput {
  source_thread_id?: string
  // The loop that publishes this app onto its boards.
  loop_id?: string
  title: string
  description?: string
  kind?: BundleKind
  entry?: string
  html?: string
  markdown?: string
}

// Saves a staged draft the user approved, as a private app or as the next
// version of its target, which keeps the sharing it already has.
export async function saveAppDraft(draft: PendingAppDraft): Promise<BundledApp> {
  const body = { draft_id: draft.draft_id, bundle_sha256: draft.bundle_sha256 }
  const saved = draft.target_app_id
    ? put<{ app: BundledApp }>(`/v1/apps/${encodeURIComponent(draft.target_app_id)}`, body)
    : post<{ app: BundledApp }>('/v1/apps', body)
  return (await saved).app
}

// Saves an artifact or document as a private app in one request, with its own
// sandboxed storage and nothing that needs approval.
export async function saveApp(input: AppBundleInput): Promise<BundledApp> {
  return (await post<{ app: BundledApp }>('/v1/apps', { ...input, capabilities: ['storage:personal'] })).app
}

export function patchApp(
  id: string,
  input: {
    title?: string
    description?: string
    icon?: AppIconSpec
    visibility?: AppVisibility
    space_id?: string | null
    member_user_ids?: string[]
    members?: AppMemberInput[]
  },
): Promise<JazApp> {
  return patch<JazApp>(`/v1/apps/${encodeURIComponent(id)}`, input)
}

export function deleteApp(id: string): Promise<{ deleted: boolean; id: string }> {
  return del<{ deleted: boolean; id: string }>(`/v1/apps/${encodeURIComponent(id)}`)
}

export function rollbackApp(id: string, version: number): Promise<{ app: JazApp; version: AppVersion }> {
  return post<{ app: JazApp; version: AppVersion }>(`/v1/apps/${encodeURIComponent(id)}/rollback`, { version })
}

export function remixApp(
  id: string,
  input: { title?: string; space_id?: string | null } = {},
): Promise<AppRemixResult> {
  return post<AppRemixResult>(`/v1/apps/${encodeURIComponent(id)}/remix`, input)
}

export function issueAppTicket(id: string, version?: number): Promise<AppTicketResponse> {
  const qs = version && version > 0 ? `?version=${version}` : ''
  return post<AppTicketResponse>(`/v1/apps/${encodeURIComponent(id)}/ticket${qs}`, {})
}

export function appContentFileUrl(ticket: AppTicketResponse, relpath: string): string {
  const cleanRel = relpath.replace(/^\/+/, '')
  const encodedRel = cleanRel.split('/').map(encodeURIComponent).join('/')
  const path = `/app-content/${encodeURIComponent(ticket.app_id)}/${encodedRel}?ticket=${encodeURIComponent(ticket.ticket)}`
  return apiUrl(path)
}

export async function fetchAppContentText(
  ticket: AppTicketResponse,
  relpath = ticket.entry_path,
  signal?: AbortSignal,
): Promise<string> {
  const url = appContentFileUrl(ticket, relpath)
  const res = await backendFetch(url, {
    method: 'GET',
    credentials: 'omit',
    referrerPolicy: 'no-referrer',
    signal,
  })
  if (!res.ok) {
    throw new Error(`Failed to load app content (${res.status})`)
  }
  return res.text()
}

export async function listAppKV(
  appId: string,
  opts: { scope?: 'personal' | 'shared'; prefix?: string } = {},
): Promise<AppKVListResponse> {
  const params = new URLSearchParams()
  if (opts.scope) params.set('scope', opts.scope)
  if (opts.prefix) params.set('prefix', opts.prefix)
  const qs = params.toString()
  return get<AppKVListResponse>(`/v1/apps/${encodeURIComponent(appId)}/kv${qs ? `?${qs}` : ''}`)
}

export async function getAppKV(
  appId: string,
  key: string,
  scope: 'personal' | 'shared' = 'personal',
): Promise<AppKVEntry | null> {
  const res = await apiFetch(
    `/v1/apps/${encodeURIComponent(appId)}/kv/${encodeURIComponent(key)}?scope=${encodeURIComponent(scope)}`,
  )
  if (res.status === 404) return null
  if (!res.ok) {
    const body = (await res.json().catch(() => null)) as { error?: string } | null
    throw new Error(body?.error || `KV get failed (${res.status})`)
  }
  return (await res.json()) as AppKVEntry
}

export async function putAppKV(
  appId: string,
  key: string,
  value: string,
  opts: { scope?: 'personal' | 'shared'; expectedRevision?: number } = {},
): Promise<AppKVEntry> {
  const headers: Record<string, string> = { 'Content-Type': 'application/json' }
  if (typeof opts.expectedRevision === 'number') {
    headers['If-Match'] = `"rev-${opts.expectedRevision}"`
  }
  const res = await apiFetch(`/v1/apps/${encodeURIComponent(appId)}/kv/${encodeURIComponent(key)}`, {
    method: 'PUT',
    headers,
    body: JSON.stringify({
      scope: opts.scope ?? 'personal',
      value,
      ...(typeof opts.expectedRevision === 'number' ? { expected_revision: opts.expectedRevision } : {}),
    }),
  })
  if (!res.ok) {
    const body = (await res.json().catch(() => null)) as { error?: string } | null
    throw new Error(body?.error || `KV save failed (${res.status})`)
  }
  return (await res.json()) as AppKVEntry
}

export async function deleteAppKV(
  appId: string,
  key: string,
  opts: { scope?: 'personal' | 'shared'; expectedRevision?: number } = {},
): Promise<{ deleted: boolean }> {
  const headers: Record<string, string> = {}
  if (typeof opts.expectedRevision === 'number') {
    headers['If-Match'] = `"rev-${opts.expectedRevision}"`
  }
  const scope = opts.scope ?? 'personal'
  const res = await apiFetch(
    `/v1/apps/${encodeURIComponent(appId)}/kv/${encodeURIComponent(key)}?scope=${encodeURIComponent(scope)}`,
    { method: 'DELETE', headers },
  )
  if (!res.ok) {
    const body = (await res.json().catch(() => null)) as { error?: string } | null
    throw new Error(body?.error || `KV delete failed (${res.status})`)
  }
  return (await res.json()) as { deleted: boolean }
}

export function completeAppAI(
  appId: string,
  input: { prompt: string; system?: string; model?: string; model_profile_id?: string; max_tokens?: number },
): Promise<AppAICompleteResult> {
  return post<AppAICompleteResult>(`/v1/apps/${encodeURIComponent(appId)}/ai/complete`, input)
}

export function callAppTool(
  appId: string,
  input: { tool: string; arguments?: Record<string, unknown>; alias?: string },
): Promise<{ tool: string; result: unknown }> {
  return post<{ tool: string; result: unknown }>(`/v1/apps/${encodeURIComponent(appId)}/tools/call`, input)
}

export function updateOrgAppPolicy(input: Partial<OrgAppPolicy>): Promise<OrgAppPolicy> {
  return put<OrgAppPolicy>('/v1/org/app-policies', input)
}

export function formatAppBytes(n: number): string {
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
  return `${(n / (1024 * 1024)).toFixed(2)} MB`
}
