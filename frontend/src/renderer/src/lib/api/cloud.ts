import { queryOptions } from '@tanstack/react-query'
import { ApiError, readAPIResponse } from '@/lib/api/response'
import { backendFetch } from '@/lib/clientRuntime'
import { keys } from '@/lib/query/keys'
import type { ModelPickerOption, PickerAgentOption } from '@/lib/modelPicker'
import {
  CLIENT_PLATFORM,
  CLIENT_PLATFORM_HEADER,
  apiEmbeddedGetUrl,
  get,
  normalizeBaseUrl,
  post,
  put,
  secureBackendConnection,
} from './client'
import type { Attachment } from './types'

export type ServerMode = 'cloud' | 'local'
export type ServerAuthMethod = 'oidc' | 'bearer' | 'none'

export interface ServerCapabilities {
  session_file_read?: boolean
  cloud_instructions?: boolean
  connected_machines?: boolean
  native_codex_exec_remote?: boolean
  spaces?: boolean
}

export interface CloudUser {
  id: string
  org_id: string
  org_name?: string
  email: string
  display_name: string
  role: string
}

export interface CloudSpaceMember {
  space_id?: string
  user_id: string
  email?: string
  display_name?: string
  role: string
}

export interface CloudSpace {
  id: string
  org_id?: string
  name: string
  created_by?: string
  created_at?: string
  member_count?: number
  members?: CloudSpaceMember[]
}

export interface ThreadUserState {
  thread_id?: string
  pinned?: boolean
  archived?: boolean
  last_read_seq?: number
  personal_sources_enabled: boolean
}

export type InstructionScope = 'org' | 'space' | 'user' | 'bot'
export type InstructionKind = 'agents' | 'soul' | 'bots' | 'memory'

export interface CloudInstructionKey {
  org_id?: string
  scope: InstructionScope
  scope_id: string
  kind: InstructionKind
}

export interface CloudInstructionDocument {
  key: CloudInstructionKey
  content: string
  version: string
  updated_by?: string
  updated_at?: string
  read_only?: boolean
}

export const INSTRUCTION_SCOPE_KINDS: Record<InstructionScope, InstructionKind[]> = {
  org: ['agents'],
  space: ['agents'],
  user: ['agents', 'soul', 'bots'],
  bot: ['bots', 'memory'],
}

export const INSTRUCTION_KIND_FILENAME: Record<InstructionKind, string> = {
  agents: 'AGENTS.md',
  soul: 'SOUL.md',
  bots: 'BOTS.md',
  memory: 'MEMORY.md',
}

export type FolderAccessMode = 'ro' | 'rw' | 'rwd'

export interface MachineFolderGrant {
  id: string
  root_path: string
  mode: FolderAccessMode
  label?: string
}

export interface MachineCapabilities {
  file_staging?: boolean
  local_exec?: boolean
  local_browser?: boolean
  computer_use?: boolean
  local_mcps?: string[]
  max_stage_bytes?: number
}

export interface ConnectedMachine {
  device_id: string
  display_name: string
  platform: string
  online: boolean
  capabilities: MachineCapabilities
  folder_grants: MachineFolderGrant[]
  last_seen_at: string
}

export interface PresignUploadInput {
  relpath: string
  content_type: string
  size_bytes: number
  sha256: string
}

export interface PresignUploadResponse {
  upload_url: string
  headers?: Record<string, string>
  object_key: string
  input_id: string
  relpath?: string
}

export interface CommitUploadInput {
  input_id: string
  relpath: string
  object_key: string
  sha256: string
  size_bytes: number
  content_type: string
}

export interface CommitUploadResponse {
  upload_seq?: number
  input_id?: string
  relpath?: string
  object_key?: string
  sha256?: string
  size_bytes?: number
  content_type?: string
  attachment?: Attachment
}

export interface CloudUsageSummary {
  month?: string
  org_spend_usd: number
  org_cap_usd?: number | null
  user_spend_usd?: number
  user_cap_usd?: number | null
  percent_used?: number
  warning?: boolean
  cap_reached?: boolean
}

export interface CloudModelProfile {
  profile_id: string
  provider: string
  display_name?: string
  agent?: string
  default_model?: string
  default_effort?: string
  models?: ModelPickerOption[]
}

export interface SpendStatus {
  level: 'warning' | 'exceeded'
  scope: 'user' | 'org'
  spentUsd: number
  capUsd: number
  percent: number
  message: string
}

const SPEND_WARNING_THRESHOLD = 0.8

export function buildOidcLoginUrl(baseUrl: string, oidcLoginUrl = '/v1/auth/login', clientRedirect?: string): string {
  const origin = normalizeBaseUrl(baseUrl)
  if (!secureBackendConnection(origin)) {
    throw new Error('Remote connections require HTTPS')
  }
  const path = oidcLoginUrl.trim() || '/v1/auth/login'
  const url = /^https?:\/\//i.test(path) ? new URL(path) : new URL(path.startsWith('/') ? path : `/${path}`, `${origin}/`)
  if (clientRedirect?.trim()) {
    url.searchParams.set('client_redirect', clientRedirect.trim())
  }
  return url.toString()
}

export function parseOidcCallbackHash(input: string): { token: string; server: string } {
  const trimmed = input.trim()
  if (!trimmed) return { token: '', server: '' }
  try {
    if (/^https?:\/\//i.test(trimmed)) {
      const parsed = new URL(trimmed)
      const hashParams = new URLSearchParams(parsed.hash.replace(/^#\??/, ''))
      const token = (hashParams.get('token') || hashParams.get('key') || parsed.searchParams.get('token') || parsed.searchParams.get('key'))?.trim() ?? ''
      const server = (hashParams.get('server') || hashParams.get('url') || parsed.searchParams.get('server') || parsed.searchParams.get('url'))?.trim() ?? ''
      return { token, server }
    }
  } catch {
    // Fall through to fragment parsing.
  }
  const params = new URLSearchParams(trimmed.replace(/^[#?]+/, ''))
  const token = (params.get('token') || params.get('key'))?.trim() ?? ''
  const server = (params.get('server') || params.get('url'))?.trim() ?? ''
  return { token, server }
}

type RawAuthMe = {
  id?: string
  user_id?: string
  org_id?: string
  org_name?: string
  email?: string
  display_name?: string
  name?: string
  role?: string
  user?: RawAuthMe
}

function normalizeCloudUser(raw: RawAuthMe): CloudUser {
  const source = raw.user ?? raw
  const id = (source.id ?? source.user_id ?? '').trim()
  const orgId = (source.org_id ?? '').trim()
  const email = (source.email ?? '').trim()
  const displayName = (source.display_name ?? source.name ?? '').trim() || email
  const role = (source.role ?? 'member').trim() || 'member'
  if (!id && !email && !orgId) {
    throw new Error('Cloud server returned an invalid user profile')
  }
  return {
    id,
    org_id: orgId,
    ...(source.org_name?.trim() ? { org_name: source.org_name.trim() } : {}),
    email,
    display_name: displayName,
    role,
  }
}

export async function fetchAuthMe(
  baseUrl?: string,
  options: { token?: string; signal?: AbortSignal; credentials?: RequestCredentials } = {},
): Promise<CloudUser> {
  if (!baseUrl) {
    return normalizeCloudUser(await get<RawAuthMe>('/v1/auth/me', { signal: options.signal }))
  }
  const origin = normalizeBaseUrl(baseUrl)
  if (!secureBackendConnection(origin)) {
    throw new Error('Remote connections require HTTPS')
  }
  const headers = new Headers({ [CLIENT_PLATFORM_HEADER]: CLIENT_PLATFORM })
  if (options.token) {
    headers.set('Authorization', `Bearer ${options.token}`)
  }
  const response = await backendFetch(`${origin}/v1/auth/me`, {
    headers,
    credentials: options.credentials,
    signal: options.signal,
  })
  return normalizeCloudUser(await readAPIResponse<RawAuthMe>(response))
}

export async function listSpaces(signal?: AbortSignal): Promise<CloudSpace[]> {
  const response = await get<{ spaces?: CloudSpace[] | null } | CloudSpace[]>('/v1/spaces', { signal })
  if (Array.isArray(response)) return response
  return response.spaces ?? []
}

export const spacesQuery = queryOptions({
  queryKey: keys.cloudSpaces,
  queryFn: ({ signal }) => listSpaces(signal),
  staleTime: 15_000,
})

export async function createSpace(input: { name: string }): Promise<CloudSpace> {
  const name = input.name.trim()
  if (!name) throw new Error('Space name is required')
  return post<CloudSpace>('/v1/spaces', { name })
}

export async function addSpaceMember(
  spaceId: string,
  input: { user_id?: string; email?: string; role?: string },
): Promise<CloudSpaceMember> {
  const payload: { user_id?: string; email?: string; role: string } = {
    role: input.role?.trim() || 'member',
  }
  if (input.email?.trim()) payload.email = input.email.trim()
  if (input.user_id?.trim()) payload.user_id = input.user_id.trim()
  if (!payload.email && !payload.user_id) {
    throw new Error('Enter a user email or ID')
  }
  return post<CloudSpaceMember>(`/v1/spaces/${encodeURIComponent(spaceId)}/members`, payload)
}

export function updateThreadUserState(
  threadId: string,
  state: Partial<Pick<ThreadUserState, 'pinned' | 'archived' | 'last_read_seq' | 'personal_sources_enabled'>>,
): Promise<ThreadUserState> {
  return put<ThreadUserState>(`/v1/threads/${encodeURIComponent(threadId)}/user-state`, state)
}

type RawInstructionDocument = {
  key?: Partial<CloudInstructionKey>
  content?: string
  body_md?: string
  version?: string
  updated_by?: string
  updated_at?: string
  read_only?: boolean
}

function normalizeInstruction(
  scope: InstructionScope,
  scopeId: string,
  kind: InstructionKind,
  raw: RawInstructionDocument,
): CloudInstructionDocument {
  return {
    key: {
      org_id: raw.key?.org_id,
      scope: raw.key?.scope ?? scope,
      scope_id: raw.key?.scope_id ?? scopeId,
      kind: raw.key?.kind ?? kind,
    },
    content: raw.content ?? raw.body_md ?? '',
    version: raw.version ?? '',
    updated_by: raw.updated_by,
    updated_at: raw.updated_at,
    read_only: raw.read_only,
  }
}

export async function getInstruction(
  scope: InstructionScope,
  scopeId: string,
  kind: InstructionKind,
  signal?: AbortSignal,
): Promise<CloudInstructionDocument> {
  try {
    const raw = await get<RawInstructionDocument>(
      `/v1/instructions/${encodeURIComponent(scope)}/${encodeURIComponent(scopeId)}/${encodeURIComponent(kind)}`,
      { signal },
    )
    return normalizeInstruction(scope, scopeId, kind, raw)
  } catch (error) {
    if (error instanceof ApiError && error.status === 404) {
      return normalizeInstruction(scope, scopeId, kind, { content: '', version: '' })
    }
    throw error
  }
}

export async function saveInstruction(
  scope: InstructionScope,
  scopeId: string,
  kind: InstructionKind,
  input: { content: string; expected_version: string },
): Promise<CloudInstructionDocument> {
  const raw = await put<RawInstructionDocument>(
    `/v1/instructions/${encodeURIComponent(scope)}/${encodeURIComponent(scopeId)}/${encodeURIComponent(kind)}`,
    {
      content: input.content,
      expected_version: input.expected_version,
    },
  )
  return normalizeInstruction(scope, scopeId, kind, raw)
}

export const cloudInstructionQuery = (scope: InstructionScope, scopeId: string, kind: InstructionKind) =>
  queryOptions({
    queryKey: keys.cloudInstruction(scope, scopeId, kind),
    queryFn: ({ signal }) => getInstruction(scope, scopeId, kind, signal),
    enabled: Boolean(scopeId),
  })

export async function listMachines(signal?: AbortSignal): Promise<ConnectedMachine[]> {
  const response = await get<{ machines?: ConnectedMachine[] | null } | ConnectedMachine[]>('/v1/machines', { signal })
  const items = Array.isArray(response) ? response : (response.machines ?? [])
  return items.map((machine) => ({
    ...machine,
    capabilities: machine.capabilities ?? {},
    folder_grants: machine.folder_grants ?? [],
  }))
}

export const machinesQuery = queryOptions({
  queryKey: keys.cloudMachines,
  queryFn: ({ signal }) => listMachines(signal),
  refetchInterval: 15_000,
})

const CLOUD_PROVIDER_META: Record<string, { label: string; logoAgent: string }> = {
  openai: { label: 'Codex', logoAgent: 'codex' },
  azure_openai: { label: 'Codex Azure', logoAgent: 'codex' },
  anthropic: { label: 'Claude', logoAgent: 'claude' },
  xai: { label: 'Grok', logoAgent: 'grok' },
  bedrock: { label: 'Claude Bedrock', logoAgent: 'claude' },
  openrouter: { label: 'OpenRouter', logoAgent: 'codex' },
}

export function cloudProfileToAgentOption(profile: CloudModelProfile): PickerAgentOption {
  const provider = profile.provider.trim().toLowerCase()
  const meta = CLOUD_PROVIDER_META[provider]
  return {
    id: profile.profile_id,
    label: profile.display_name?.trim() || meta?.label || profile.provider || profile.profile_id,
    logoAgent: profile.agent?.trim() || meta?.logoAgent || 'codex',
    location: 'cloud',
    provider,
  }
}

export async function listCloudModelProfiles(signal?: AbortSignal): Promise<CloudModelProfile[]> {
  try {
    const response = await get<{ profiles?: CloudModelProfile[] | null; model_profiles?: CloudModelProfile[] | null } | CloudModelProfile[]>('/v1/models', { signal })
    if (Array.isArray(response)) return response
    return response.profiles ?? response.model_profiles ?? []
  } catch (error) {
    if (error instanceof ApiError && error.status === 404) return []
    throw error
  }
}

export const cloudModelProfilesQuery = queryOptions({
  queryKey: keys.cloudModelProfiles,
  queryFn: ({ signal }) => listCloudModelProfiles(signal),
  staleTime: 60_000,
  retry: false,
})

export function presignUpload(runId: string, input: PresignUploadInput, signal?: AbortSignal): Promise<PresignUploadResponse> {
  return post<PresignUploadResponse>(`/v1/runs/${encodeURIComponent(runId)}/uploads/presign`, input, signal)
}

export function commitUpload(runId: string, input: CommitUploadInput, signal?: AbortSignal): Promise<CommitUploadResponse> {
  return post<CommitUploadResponse>(`/v1/runs/${encodeURIComponent(runId)}/uploads/commit`, input, signal)
}

export async function sha256Hex(data: ArrayBuffer): Promise<string> {
  const digest = await crypto.subtle.digest('SHA-256', data)
  return Array.from(new Uint8Array(digest), (byte) => byte.toString(16).padStart(2, '0')).join('')
}

export async function uploadCloudFile(
  runId: string,
  file: File,
  options: { relpath?: string; signal?: AbortSignal; onProgress?: (fraction: number) => void } = {},
): Promise<Attachment> {
  const relpath = options.relpath?.trim() || file.name
  const contentType = file.type || 'application/octet-stream'
  const buffer = await file.arrayBuffer()
  const sha256 = await sha256Hex(buffer)
  options.onProgress?.(0.1)

  const presigned = await presignUpload(
    runId,
    { relpath, content_type: contentType, size_bytes: file.size, sha256 },
    options.signal,
  )
  options.onProgress?.(0.25)

  const uploadHeaders = new Headers(presigned.headers)
  if (!uploadHeaders.has('Content-Type')) {
    uploadHeaders.set('Content-Type', contentType)
  }
  const putRes = await fetch(presigned.upload_url, {
    method: 'PUT',
    headers: uploadHeaders,
    body: file,
    signal: options.signal,
  })
  if (!putRes.ok) {
    throw new ApiError(putRes.status, `Upload storage rejected file (${putRes.status})`)
  }
  options.onProgress?.(0.85)

  const committed = await commitUpload(
    runId,
    {
      input_id: presigned.input_id,
      relpath: presigned.relpath || relpath,
      object_key: presigned.object_key,
      sha256,
      size_bytes: file.size,
      content_type: contentType,
    },
    options.signal,
  )
  options.onProgress?.(1)

  if (committed.attachment) {
    return committed.attachment
  }
  const finalRelpath = committed.relpath || presigned.relpath || relpath
  return {
    id: committed.input_id || presigned.input_id || committed.object_key || presigned.object_key,
    name: file.name,
    uri: cloudRunFileUrl(runId, finalRelpath),
    mime_type: contentType,
    size: file.size,
  }
}

export function cloudRunFileUrl(runId: string, relpath: string): string {
  const encodedPath = relpath
    .replace(/^\/+/, '')
    .split('/')
    .map((segment) => encodeURIComponent(segment))
    .join('/')
  return apiEmbeddedGetUrl(`/v1/runs/${encodeURIComponent(runId)}/files/${encodedPath}`)
}

export function fetchUsageSummary(signal?: AbortSignal): Promise<CloudUsageSummary> {
  return get<CloudUsageSummary>('/v1/usage/summary', { signal })
}

export const usageSummaryQuery = queryOptions({
  queryKey: keys.cloudUsageSummary,
  queryFn: ({ signal }) => fetchUsageSummary(signal),
  staleTime: 60_000,
  retry: false,
})

export function evaluateSpendStatus(summary: CloudUsageSummary | null | undefined): SpendStatus | null {
  if (!summary) return null

  const userCap = typeof summary.user_cap_usd === 'number' && summary.user_cap_usd > 0 ? summary.user_cap_usd : 0
  const userSpend = typeof summary.user_spend_usd === 'number' ? summary.user_spend_usd : 0
  const orgCap = typeof summary.org_cap_usd === 'number' && summary.org_cap_usd > 0 ? summary.org_cap_usd : 0
  const orgSpend = typeof summary.org_spend_usd === 'number' ? summary.org_spend_usd : 0

  const userRatio = userCap > 0 ? userSpend / userCap : 0
  const orgRatio = orgCap > 0 ? orgSpend / orgCap : 0
  const useUser = userCap > 0 && (userRatio >= orgRatio || orgCap === 0)
  const capUsd = useUser ? userCap : orgCap
  const spentUsd = useUser ? userSpend : orgSpend
  const scope: 'user' | 'org' = useUser ? 'user' : 'org'
  const ratio = capUsd > 0 ? spentUsd / capUsd : (summary.percent_used ?? 0) / 100
  const percent = Math.round(ratio * 100)

  if (summary.cap_reached || (capUsd > 0 && spentUsd >= capUsd)) {
    const label = scope === 'user' ? 'Your monthly spend cap' : 'Organization monthly spend cap'
    return {
      level: 'exceeded',
      scope,
      spentUsd,
      capUsd,
      percent: Math.max(100, percent),
      message: capUsd > 0
        ? `${label} reached ($${spentUsd.toFixed(2)} / $${capUsd.toFixed(2)}). Contact an org admin to raise it.`
        : `${label} reached. Contact an org admin to continue.`,
    }
  }

  if (summary.warning || (capUsd > 0 && ratio >= SPEND_WARNING_THRESHOLD)) {
    const label = scope === 'user' ? 'Monthly spend' : 'Organization spend'
    return {
      level: 'warning',
      scope,
      spentUsd,
      capUsd,
      percent,
      message: capUsd > 0
        ? `${label} at ${percent}% ($${spentUsd.toFixed(2)} of $${capUsd.toFixed(2)} cap).`
        : `${label} is approaching the monthly cap.`,
    }
  }

  return null
}

export function isSpendCapError(error: unknown): boolean {
  if (error instanceof ApiError && error.status === 402) return true
  if (error instanceof Error && /\b402\b|spend cap/i.test(error.message)) return true
  return false
}

function shellQuote(value: string): string {
  if (/^[a-zA-Z0-9._/:=@-]+$/.test(value)) return value
  return `'${value.replace(/'/g, `'\\''`)}'`
}

export function buildConnectCloudCommand(input: {
  cloudUrl: string
  deviceId: string
  displayName: string
  grants: MachineFolderGrant[]
  allowExec: boolean
}): string {
  const parts = [
    'jaz connect-cloud',
    `--url ${shellQuote(normalizeBaseUrl(input.cloudUrl) || input.cloudUrl)}`,
    '--token "$JAZ_CLOUD_TOKEN"',
    `--device-id ${shellQuote(input.deviceId.trim() || 'workstation-1')}`,
  ]
  if (input.displayName.trim()) {
    parts.push(`--name ${shellQuote(input.displayName.trim())}`)
  }
  for (const grant of input.grants) {
    const path = grant.root_path.trim()
    const id = grant.id.trim()
    if (!path || !id) continue
    parts.push(`--grant ${shellQuote(`${id}=${grant.mode}:${path}`)}`)
  }
  if (input.allowExec) {
    parts.push('--allow-exec')
  }
  return parts.join(' \\\n  ')
}
