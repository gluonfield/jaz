import { queryOptions } from '@tanstack/react-query'
import { ApiError } from '@/lib/api/response'
import type { ModelPickerOption, PickerAgentOption } from '@/lib/modelPicker'
import { modelSuggestionFor } from '@/lib/modelSuggestion'
import { reasoningEffortLabel } from '@/lib/reasoningEfforts'
import { keys } from '@/lib/query/keys'
import {
  apiFetch as cloudFetch,
  apiSessionRevision,
  del as cloudDel,
  get as cloudGet,
  onApiSessionChange as onCloudSessionChange,
  patch as cloudPatch,
  post as cloudPost,
} from './client'
import type { ConnectedMachine } from './serverTypes'
import type { Attachment, ReasoningEffortOption, Session, SessionEvent, SessionMessages } from './types'

export type RawCloudModelOption = string | ModelPickerOption

export interface CloudModelProfile {
  profile_id: string
  provider: string
  base_url?: string
  display_name?: string
  agent?: string
  scope?: 'org' | 'user'
  account_label?: string
  default_model?: string
  default_effort?: string
  models?: ModelPickerOption[]
}

type RawCloudModelProfile = Omit<CloudModelProfile, 'models'> & {
  models?: RawCloudModelOption[] | null
}

export interface CloudThread {
  id: string
  org_id?: string
  owner_user_id?: string
  space_id?: string | null
  run_id: string
  visibility: 'private' | 'space'
  source_type?: string
  source_id?: string
  title: string
  status: string
  turn_epoch?: number
  turn_actor_user_id?: string | null
  model_profile_id?: string
  model?: string
  reasoning_effort?: string
  runtime_ref_json?: Record<string, unknown> | null
  personal_sources_enabled?: boolean
  pinned?: boolean
  archived?: boolean
  unread?: boolean
  created_at?: string
  updated_at: string
}

export interface CloudTurn {
  id: string
  thread_id: string
  org_id?: string
  actor_user_id?: string
  client_request_id?: string
  turn_epoch?: number
  status: string
  created_at?: string
}

export interface CloudSessionEvent {
  thread_id: string
  seq: number
  org_id?: string
  turn_epoch?: number
  actor_user_id?: string | null
  type: string
  payload?: Record<string, unknown> | null
  created_at: string
}

export interface CreateCloudThreadInput {
  title?: string
  space_id?: string
  model_profile_id?: string
  model?: string
  reasoning_effort?: string
  executor_target?: string
}

export interface CreateCloudTurnInput {
  text: string
  client_request_id?: string
  model_profile_id?: string
  executor_target?: string
  require_sandbox?: boolean
  uploads?: Attachment[]
  signal?: AbortSignal
}

const CLOUD_PROVIDER_META: Record<string, { label: string; logoAgent: string }> = {
  openai: { label: 'Codex', logoAgent: 'codex' },
  azure_openai: { label: 'Codex Azure', logoAgent: 'codex' },
  codex_oauth: { label: 'Codex', logoAgent: 'codex' },
  anthropic: { label: 'Claude', logoAgent: 'claude' },
  bedrock: { label: 'Claude Bedrock', logoAgent: 'claude' },
  claude_oauth: { label: 'Claude', logoAgent: 'claude' },
  antigravity_oauth: { label: 'Antigravity', logoAgent: 'antigravity' },
  kimi_oauth: { label: 'Kimi', logoAgent: 'kimi' },
  xai: { label: 'Grok', logoAgent: 'grok' },
  vertex: { label: 'Vertex AI', logoAgent: 'gemini' },
  openrouter: { label: 'OpenRouter', logoAgent: 'codex' },
}

function normalizeModelOption(option: RawCloudModelOption): ModelPickerOption | null {
  if (typeof option === 'string') {
    const value = option.trim()
    if (!value) return null
    return {
      value,
      label: value.includes('/') ? value.slice(value.indexOf('/') + 1) : value,
    }
  }
  if (!option || typeof option !== 'object' || typeof option.value !== 'string' || !option.value.trim()) {
    return null
  }
  if (!option.reasoning) {
    return { ...option, value: option.value.trim(), label: option.label?.trim() || option.value.trim() }
  }
  return {
    ...option,
    value: option.value.trim(),
    label: option.label?.trim() || option.value.trim(),
    reasoning: {
      ...option.reasoning,
      status: option.reasoning.status ?? 'ready',
    },
  }
}

function normalizeCloudModelProfile(raw: RawCloudModelProfile): CloudModelProfile {
  const provider = (raw.provider ?? '').trim().toLowerCase()
  const meta = CLOUD_PROVIDER_META[provider]
  const fallbackEffort = raw.default_effort?.trim() ?? ''
  const parsedModels = Array.isArray(raw.models)
    ? raw.models
        .map((item) => normalizeModelOption(item))
        .filter((item): item is ModelPickerOption => item !== null)
    : []
  const defaultModelOpt = raw.default_model ? normalizeModelOption(raw.default_model) : null
  const models: ModelPickerOption[] = parsedModels.length > 0
    ? parsedModels
    : defaultModelOpt
      ? [defaultModelOpt]
      : []
  const defaultModel = raw.default_model?.trim() || models[0]?.value || ''
  const defaultEffort = fallbackEffort || models.find((m) => m.value === defaultModel)?.reasoning?.default_effort || ''
  return {
    ...raw,
    provider,
    display_name: raw.display_name?.trim() || meta?.label || raw.provider || raw.profile_id,
    agent: raw.agent?.trim() || meta?.logoAgent || '',
    default_model: defaultModel,
    default_effort: defaultEffort,
    models,
  }
}

export function cloudProfileToAgentOption(profile: CloudModelProfile): PickerAgentOption {
  const provider = profile.provider.trim().toLowerCase()
  const meta = CLOUD_PROVIDER_META[provider]
  return {
    id: profile.profile_id,
    label: profile.display_name?.trim() || meta?.label || profile.provider || profile.profile_id,
    logoAgent: profile.agent?.trim() || meta?.logoAgent || '',
    provider,
  }
}

export interface ResolvedCloudModelSelection {
  profile?: CloudModelProfile
  agentItems: PickerAgentOption[]
  agents: string[]
  profileId: string
  model: string
  modelSuggestions: ModelPickerOption[]
  effort: string
  effortOptions: ReasoningEffortOption[]
}

export function findCloudModelProfile(
  profiles: CloudModelProfile[],
  selector?: string,
  fallbackAgent?: string,
): CloudModelProfile | undefined {
  if (profiles.length === 0) return undefined
  const trimmed = selector?.trim() ?? ''
  if (trimmed) {
    const exact = profiles.find((profile) => profile.profile_id === trimmed)
    if (exact) return exact
    const lowered = trimmed.toLowerCase()
    const byProvider = profiles.find((profile) => profile.provider === lowered)
    if (byProvider) return byProvider
    const byAgent = profiles.find((profile) => (profile.agent ?? '').toLowerCase() === lowered)
    if (byAgent) return byAgent
  }
  const agentTrimmed = fallbackAgent?.trim().toLowerCase() ?? ''
  if (agentTrimmed) {
    const byAgent = profiles.find((profile) => (profile.agent ?? '').toLowerCase() === agentTrimmed)
    if (byAgent) return byAgent
  }
  return profiles[0]
}

export function resolveCloudModelSelection(
  profiles: CloudModelProfile[],
  input: { profileId?: string; agent?: string; model?: string; effort?: string } = {},
): ResolvedCloudModelSelection {
  const agentItems = profiles.map(cloudProfileToAgentOption)
  const agents = agentItems.map((item) => item.id)
  const profile = findCloudModelProfile(profiles, input.profileId, input.agent)
  const modelSuggestions = profile?.models ?? []
  const requestedModel = input.model?.trim() ?? ''
  const storedModelValid = Boolean(
    requestedModel && (modelSuggestions.length === 0 || modelSuggestions.some((option) => option.value === requestedModel)),
  )
  const model = storedModelValid
    ? requestedModel
    : (profile?.default_model || modelSuggestions[0]?.value || requestedModel)
  const selectedSuggestion = modelSuggestionFor(modelSuggestions, model)
  const effortOptions = (selectedSuggestion?.reasoning?.efforts ?? []).map((value) => ({
    value,
    label: reasoningEffortLabel(value, []),
  }))
  const requestedEffort = input.effort?.trim() || profile?.default_effort || ''
  const effort = effortOptions.some((option) => option.value === requestedEffort)
    ? requestedEffort
    : selectedSuggestion?.reasoning?.default_effort ?? effortOptions[0]?.value ?? ''
  return {
    profile,
    agentItems,
    agents,
    profileId: profile?.profile_id ?? '',
    model,
    modelSuggestions,
    effort,
    effortOptions,
  }
}

export async function listCloudModelProfiles(signal?: AbortSignal): Promise<CloudModelProfile[]> {
  try {
    const response = await cloudGet<
      | { models?: RawCloudModelProfile[] | null; profiles?: RawCloudModelProfile[] | null; model_profiles?: RawCloudModelProfile[] | null }
      | RawCloudModelProfile[]
    >('/v1/models', { signal })
    const raw = Array.isArray(response)
      ? response
      : (response.models ?? response.profiles ?? response.model_profiles ?? [])
    return raw.filter((item) => Boolean(item?.profile_id)).map(normalizeCloudModelProfile)
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

export async function listMachines(signal?: AbortSignal): Promise<ConnectedMachine[]> {
  const response = await cloudGet<{ machines?: ConnectedMachine[] | null } | ConnectedMachine[]>('/v1/machines', { signal })
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

function mapCloudThreadStatus(status: string | undefined): Session['status'] {
  switch ((status ?? '').toLowerCase()) {
    case 'running':
    case 'queued':
      return 'running'
    case 'error':
    case 'failed':
      return 'error'
    case 'interrupted':
      return 'interrupted'
    default:
      return 'idle'
  }
}

function inferCloudThreadAgent(modelProfileId?: string, model?: string): string {
  const profile = (modelProfileId ?? '').trim().toLowerCase()
  if (profile.startsWith('acp:')) {
    const agent = profile.slice(4).trim()
    if (agent) return agent
  }
  const m = (model ?? '').trim().toLowerCase()
  if (m.includes('gemini') || m.startsWith('google/') || profile.includes('vertex')) return 'gemini'
  if (m.includes('claude') || profile.includes('anthropic')) return 'claude'
  if (m.includes('grok') || profile.includes('xai')) return 'grok'
  if (m.includes('kimi') || profile.includes('moonshot')) return 'kimi'
  if (m.includes('gpt') || m.includes('codex') || profile.includes('openai')) return 'codex'
  return ''
}

export function cloudThreadToSession(thread: CloudThread): Session {
  const ref = thread.runtime_ref_json && typeof thread.runtime_ref_json === 'object' ? thread.runtime_ref_json : {}
  const updatedAt = thread.updated_at || thread.created_at || new Date().toISOString()
  const agent = (typeof ref.agent === 'string' && ref.agent.trim()) || inferCloudThreadAgent(thread.model_profile_id, thread.model)
  return {
    id: thread.id,
    slug: thread.id,
    title: thread.title || undefined,
    source_type: thread.source_type || undefined,
    status: mapCloudThreadStatus(thread.status),
    runtime: 'acp',
    runtime_ref: {
      type: 'acp',
      agent,
      ...(typeof ref.cwd === 'string' ? { cwd: ref.cwd } : {}),
    },
    model_provider: thread.model_profile_id || undefined,
    model: thread.model || undefined,
    reasoning_effort: thread.reasoning_effort || undefined,
    space_id: thread.space_id || undefined,
    visibility: thread.visibility === 'space' ? 'space' : 'private',
    run_id: thread.run_id || thread.id,
    personal_sources_enabled: thread.personal_sources_enabled,
    pinned: Boolean(thread.pinned),
    archived: Boolean(thread.archived),
    unread: Boolean(thread.unread),
    backend: 'cloud',
    created_at: thread.created_at || updatedAt,
    updated_at: updatedAt,
    last_attention_at: updatedAt,
  }
}

export async function listCloudThreads(spaceId?: string, signal?: AbortSignal): Promise<CloudThread[]> {
  const query = spaceId ? `?space_id=${encodeURIComponent(spaceId)}` : ''
  const response = await cloudGet<{ threads?: CloudThread[] | null } | CloudThread[]>(`/v1/threads${query}`, { signal })
  return Array.isArray(response) ? response : (response.threads ?? [])
}

export async function createCloudThread(input: CreateCloudThreadInput): Promise<CloudThread> {
  return cloudPost<CloudThread>('/v1/threads', {
    ...(input.title ? { title: input.title } : {}),
    ...(input.space_id ? { space_id: input.space_id } : {}),
    ...(input.model_profile_id ? { model_profile_id: input.model_profile_id } : {}),
    ...(input.model ? { model: input.model } : {}),
    ...(input.reasoning_effort ? { reasoning_effort: input.reasoning_effort } : {}),
    ...(input.executor_target ? { executor_target: input.executor_target } : {}),
  })
}

export async function getCloudThread(threadId: string, signal?: AbortSignal): Promise<CloudThread> {
  return cloudGet<CloudThread>(`/v1/threads/${encodeURIComponent(threadId)}`, { signal })
}

export async function patchCloudThread(
  threadId: string,
  patch: { title?: string; space_id?: string },
): Promise<CloudThread> {
  return cloudPatch<CloudThread>(`/v1/threads/${encodeURIComponent(threadId)}`, patch)
}

export async function deleteCloudThread(threadId: string): Promise<{ deleted: boolean; id: string }> {
  return cloudDel<{ deleted: boolean; id: string }>(`/v1/threads/${encodeURIComponent(threadId)}`)
}

export async function cancelCloudThread(threadId: string): Promise<{ ok: boolean }> {
  await cloudPost<unknown>(`/v1/threads/${encodeURIComponent(threadId)}/cancel`, {})
  return { ok: true }
}

export async function steerCloudThread(
  threadId: string,
  text: string,
  mode: 'queue' | 'steer' = 'queue',
): Promise<{ ok: boolean }> {
  await cloudPost<unknown>(`/v1/threads/${encodeURIComponent(threadId)}/steer`, { text, mode })
  return { ok: true }
}

export function createCloudTurn(
  threadId: string,
  input: CreateCloudTurnInput,
): Promise<{ turn: CloudTurn; replayed: boolean }> {
  const clientRequestId =
    input.client_request_id?.trim() ||
    (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function'
      ? crypto.randomUUID()
      : `${Date.now()}-${Math.random().toString(36).slice(2)}`)
  return cloudPost<{ turn: CloudTurn; replayed: boolean }>(
    `/v1/threads/${encodeURIComponent(threadId)}/turns`,
    {
      client_request_id: clientRequestId,
      text: input.text,
      prompt: input.text,
      ...(input.model_profile_id ? { model_profile_id: input.model_profile_id } : {}),
      ...(input.executor_target ? { executor_target: input.executor_target } : {}),
      ...(input.require_sandbox !== undefined ? { require_sandbox: input.require_sandbox } : {}),
      ...(input.uploads?.length ? { uploads: input.uploads } : {}),
    },
    input.signal,
  )
}

export async function fetchCloudThreadMessages(threadId: string, signal?: AbortSignal): Promise<SessionMessages> {
  const data = await cloudGet<SessionMessages>(`/v1/threads/${encodeURIComponent(threadId)}/messages`, { signal })
  return {
    ...data,
    messages: data.messages ?? [],
    events: data.events ?? [],
  }
}

function parseSessionEventFrame(frame: string): SessionEvent | null {
  const dataLines: string[] = []
  for (const line of frame.split('\n')) {
    if (!line || line.startsWith(':')) continue
    if (line.startsWith('data:')) {
      dataLines.push(line.slice(5).trimStart())
    }
  }
  if (dataLines.length === 0) return null
  try {
    const parsed = JSON.parse(dataLines.join('\n')) as Partial<SessionEvent>
    if (typeof parsed.type !== 'string') return null
    return parsed as SessionEvent
  } catch {
    return null
  }
}

export function openCloudThreadEvents(
  threadId: string,
  afterSeq: number,
  onEvent: (event: SessionEvent) => void,
  onConnection?: (connected: boolean) => void,
): () => void {
  let closed = false
  let lastSeq = afterSeq
  let retryTimer: ReturnType<typeof setTimeout> | undefined
  let activeController: AbortController | null = null
  const revision = apiSessionRevision()

  async function connect(): Promise<void> {
    if (closed || apiSessionRevision() !== revision) return
    const controller = new AbortController()
    activeController = controller
    const params = new URLSearchParams({ format: 'session' })
    if (lastSeq > 0) params.set('after_seq', String(lastSeq))
    try {
      const response = await cloudFetch(
        `/v1/threads/${encodeURIComponent(threadId)}/events?${params.toString()}`,
        {
          headers: { Accept: 'text/event-stream' },
          signal: controller.signal,
        },
      )
      if (!response.ok || !response.body) {
        if (response.status === 401 || response.status === 403 || response.status === 404) {
          onConnection?.(false)
          return
        }
        throw new Error(`Cloud event stream failed (${response.status})`)
      }
      if (closed || apiSessionRevision() !== revision) return
      onConnection?.(true)
      const reader = response.body.getReader()
      const decoder = new TextDecoder()
      let buffer = ''
      for (;;) {
        const { done, value } = await reader.read()
        if (done) break
        if (closed || apiSessionRevision() !== revision) return
        buffer = (buffer + decoder.decode(value, { stream: true })).replace(/\r\n/g, '\n')
        let sep = buffer.indexOf('\n\n')
        while (sep !== -1) {
          const parsed = parseSessionEventFrame(buffer.slice(0, sep).replace(/\r/g, ''))
          buffer = buffer.slice(sep + 2)
          if (parsed) {
            if (typeof parsed.seq === 'number' && parsed.seq > lastSeq) lastSeq = parsed.seq
            onEvent(parsed)
          }
          sep = buffer.indexOf('\n\n')
        }
      }
    } catch {
      // Reconnect below unless closed.
    }
    if (!closed && apiSessionRevision() === revision) {
      onConnection?.(false)
      retryTimer = setTimeout(() => void connect(), 2_000)
    }
  }

  const unsubscribe = onCloudSessionChange(close)
  function close(): void {
    closed = true
    clearTimeout(retryTimer)
    activeController?.abort()
    activeController = null
    unsubscribe()
  }

  void connect()
  return close
}
