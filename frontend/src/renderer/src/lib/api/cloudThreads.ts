import { queryOptions } from '@tanstack/react-query'
import { ApiError } from '@/lib/api/response'
import type { ModelPickerOption, PickerAgentOption } from '@/lib/modelPicker'
import { keys } from '@/lib/query/keys'
import {
  apiFetch as cloudFetch,
  apiSessionRevision,
  get as cloudGet,
  onApiSessionChange as onCloudSessionChange,
  post as cloudPost,
} from './client'
import type { ConnectedMachine } from './serverTypes'
import type { Attachment, ChatMessage, MessageBlock, Session, SessionEvent, SessionMessages } from './types'

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
  codex_oauth: { label: 'Codex (Personal)', logoAgent: 'codex' },
  anthropic: { label: 'Claude', logoAgent: 'claude' },
  bedrock: { label: 'Claude Bedrock', logoAgent: 'claude' },
  claude_oauth: { label: 'Claude Code (Personal)', logoAgent: 'claude' },
  antigravity_oauth: { label: 'Antigravity (Personal)', logoAgent: 'antigravity' },
  kimi_oauth: { label: 'Kimi (Personal)', logoAgent: 'kimi' },
  xai: { label: 'Grok', logoAgent: 'grok' },
  vertex: { label: 'Vertex AI', logoAgent: 'codex' },
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

export function cloudThreadToSession(thread: CloudThread): Session {
  const ref = thread.runtime_ref_json && typeof thread.runtime_ref_json === 'object' ? thread.runtime_ref_json : {}
  const updatedAt = thread.updated_at || thread.created_at || new Date().toISOString()
  return {
    id: thread.id,
    slug: thread.id,
    title: thread.title || undefined,
    source_type: thread.source_type || undefined,
    status: mapCloudThreadStatus(thread.status),
    runtime: 'acp',
    runtime_ref: {
      type: 'acp',
      agent: typeof ref.agent === 'string' && ref.agent ? ref.agent : '',
      ...(typeof ref.cwd === 'string' ? { cwd: ref.cwd } : {}),
    },
    model_provider: thread.model_profile_id || undefined,
    model: thread.model || undefined,
    reasoning_effort: thread.reasoning_effort || undefined,
    space_id: thread.space_id || undefined,
    visibility: thread.visibility === 'space' || thread.space_id ? 'space' : 'private',
    run_id: thread.run_id || thread.id,
    personal_sources_enabled: thread.personal_sources_enabled,
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

export function parseCloudSseText(rawText: string): CloudSessionEvent[] {
  const events: CloudSessionEvent[] = []
  const normalized = rawText.replace(/\r\n/g, '\n')
  for (const block of normalized.split('\n\n')) {
    const parsed = parseSseFrame(block)
    if (parsed) events.push(parsed)
  }
  return events
}

function parseSseFrame(frame: string): CloudSessionEvent | null {
  let eventName = ''
  let eventId = ''
  const dataLines: string[] = []
  for (const line of frame.split('\n')) {
    if (!line || line.startsWith(':')) continue
    if (line.startsWith('event:')) {
      eventName = line.slice(6).trim()
    } else if (line.startsWith('id:')) {
      eventId = line.slice(3).trim()
    } else if (line.startsWith('data:')) {
      dataLines.push(line.slice(5).trimStart())
    }
  }
  if (dataLines.length === 0) return null
  try {
    const parsed = JSON.parse(dataLines.join('\n')) as Partial<CloudSessionEvent>
    const seq = typeof parsed.seq === 'number' ? parsed.seq : Number.parseInt(eventId, 10) || 0
    return {
      thread_id: parsed.thread_id ?? '',
      seq,
      org_id: parsed.org_id,
      turn_epoch: parsed.turn_epoch,
      actor_user_id: parsed.actor_user_id,
      type: parsed.type || eventName || 'message',
      payload: parsed.payload ?? null,
      created_at: parsed.created_at || new Date().toISOString(),
    }
  } catch {
    return null
  }
}

function isCloudUserEvent(type: string): boolean {
  return type === 'user_message' || type === 'turn.user' || type === 'turn_user' || type === 'turn.started' || type === 'turn_started'
}

function isCloudAssistantDeltaEvent(type: string): boolean {
  return type === 'assistant_delta' || type === 'assistant.delta' || type === 'agent_message_chunk'
}

function isCloudAssistantMessageEvent(type: string): boolean {
  return type === 'assistant_message' || type === 'assistant.message' || type === 'turn.assistant' || type === 'turn_assistant'
}

function cloudTerminalEventState(type: string): 'idle' | 'failed' | 'cancelled' | null {
  if (type === 'turn_completed' || type === 'turn.completed') return 'idle'
  if (type === 'turn_failed' || type === 'turn.failed') return 'failed'
  if (type === 'turn_interrupted' || type === 'turn.interrupted') return 'cancelled'
  return null
}

export function cloudEventToSessionEvent(threadId: string, ev: CloudSessionEvent): SessionEvent {
  const payload = ev.payload && typeof ev.payload === 'object' ? ev.payload : {}
  const base: SessionEvent = {
    seq: ev.seq,
    session_id: ev.thread_id || threadId,
    type: ev.type,
    at: ev.created_at,
  }
  if (ev.type === 'acp' && payload.acp && typeof payload.acp === 'object') {
    return { ...base, ...(payload as Partial<SessionEvent>) }
  }
  if (isCloudAssistantDeltaEvent(ev.type) || isCloudAssistantMessageEvent(ev.type)) {
    const text = typeof payload.text === 'string'
      ? payload.text
      : typeof payload.delta === 'string'
        ? payload.delta
        : ''
    return {
      ...base,
      type: 'acp',
      acp: {
        id: threadId,
        slug: threadId,
        agent: typeof payload.agent === 'string' ? payload.agent : '',
        session_id: threadId,
        state: 'running',
        assistant: text,
        last_event_at: ev.created_at,
      },
    }
  }
  const terminalState = cloudTerminalEventState(ev.type)
  if (terminalState) {
    const error = typeof payload.error === 'string' ? payload.error : undefined
    return {
      ...base,
      type: 'acp',
      acp: {
        id: threadId,
        slug: threadId,
        agent: typeof payload.agent === 'string' ? payload.agent : '',
        session_id: threadId,
        state: terminalState,
        ...(error ? { error } : {}),
        last_event_at: ev.created_at,
      },
    }
  }
  return { ...base, ...(payload as Partial<SessionEvent>), seq: ev.seq, session_id: ev.thread_id || threadId, at: ev.created_at }
}

export function projectCloudThreadMessages(thread: CloudThread, rawEvents: CloudSessionEvent[]): SessionMessages {
  const session = cloudThreadToSession(thread)
  const messages: ChatMessage[] = []
  const events: SessionEvent[] = []
  let latestSeq = 0
  let acpState = session.status === 'running' ? 'running' : 'idle'
  let acpError: string | undefined
  let pendingAssistantText = ''
  let pendingAssistantSeq = 0
  let pendingAssistantAt = ''

  const flushPendingAssistant = () => {
    if (!pendingAssistantText) return
    messages.push({
      seq: pendingAssistantSeq,
      role: 'assistant',
      content: pendingAssistantText,
      blocks: [{ type: 'text', text: pendingAssistantText }],
      created_at: pendingAssistantAt,
    })
    pendingAssistantText = ''
    pendingAssistantSeq = 0
    pendingAssistantAt = ''
  }

  for (const ev of rawEvents) {
    if (ev.seq > latestSeq) latestSeq = ev.seq
    const payload = ev.payload && typeof ev.payload === 'object' ? ev.payload : {}
    const text = typeof payload.text === 'string'
      ? payload.text
      : typeof payload.prompt === 'string'
        ? payload.prompt
        : typeof payload.content === 'string'
          ? payload.content
          : typeof payload.delta === 'string'
            ? payload.delta
            : ''

    if (isCloudUserEvent(ev.type) && text) {
      flushPendingAssistant()
      acpState = 'running'
      acpError = undefined
      const blocks: MessageBlock[] = [{ type: 'text', text }]
      messages.push({
        seq: ev.seq,
        role: 'user',
        from_user: true,
        actor_user_id: ev.actor_user_id ?? undefined,
        actor_display_name: typeof payload.actor_display_name === 'string' ? payload.actor_display_name : undefined,
        actor_email: typeof payload.actor_email === 'string' ? payload.actor_email : undefined,
        content: text,
        blocks,
        created_at: ev.created_at,
      })
      continue
    }

    if (isCloudAssistantDeltaEvent(ev.type)) {
      if (text) {
        acpState = 'running'
        pendingAssistantText += text
        pendingAssistantSeq = ev.seq
        pendingAssistantAt = ev.created_at
      }
      continue
    }

    if (isCloudAssistantMessageEvent(ev.type) && text) {
      pendingAssistantText = ''
      messages.push({
        seq: ev.seq,
        role: 'assistant',
        content: text,
        blocks: [{ type: 'text', text }],
        created_at: ev.created_at,
      })
      continue
    }

    const terminalState = cloudTerminalEventState(ev.type)
    if (terminalState) {
      flushPendingAssistant()
      acpState = terminalState
      acpError = typeof payload.error === 'string' && payload.error.trim() ? payload.error.trim() : undefined
    }

    events.push(cloudEventToSessionEvent(thread.id, ev))
  }

  let acpAssistant: string | undefined
  if (acpState === 'running' && pendingAssistantText) {
    acpAssistant = pendingAssistantText
  } else {
    flushPendingAssistant()
  }

  return {
    session,
    messages,
    events,
    has_earlier: false,
    history_revision: Math.max(1, latestSeq),
    latest_event_seq: latestSeq,
    acp_state: acpState,
    ...(acpAssistant ? { acp_assistant: acpAssistant } : {}),
    ...(acpError ? { acp_error: acpError } : {}),
  }
}

export async function fetchCloudThreadMessages(threadId: string, signal?: AbortSignal): Promise<SessionMessages> {
  const [thread, eventsRes] = await Promise.all([
    getCloudThread(threadId, signal),
    cloudFetch(`/v1/threads/${encodeURIComponent(threadId)}/events?once=1`, {
      headers: { Accept: 'text/event-stream' },
      signal,
    }),
  ])
  const rawEvents = eventsRes.ok ? parseCloudSseText(await eventsRes.text()) : []
  return projectCloudThreadMessages(thread, rawEvents)
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
  let streamedAssistant = ''
  const revision = apiSessionRevision()

  function emitCloudEvent(parsed: CloudSessionEvent): void {
    if (isCloudUserEvent(parsed.type)) {
      streamedAssistant = ''
    } else if (isCloudAssistantDeltaEvent(parsed.type)) {
      const payload = parsed.payload && typeof parsed.payload === 'object' ? parsed.payload : {}
      const chunk = typeof payload.text === 'string'
        ? payload.text
        : typeof payload.delta === 'string'
          ? payload.delta
          : ''
      streamedAssistant += chunk
      onEvent(cloudEventToSessionEvent(threadId, {
        ...parsed,
        payload: { ...payload, text: streamedAssistant },
      }))
      return
    } else if (cloudTerminalEventState(parsed.type)) {
      streamedAssistant = ''
    }
    onEvent(cloudEventToSessionEvent(threadId, parsed))
  }

  async function connect(): Promise<void> {
    if (closed || apiSessionRevision() !== revision) return
    const controller = new AbortController()
    activeController = controller
    const suffix = lastSeq > 0 ? `?after_seq=${lastSeq}` : ''
    try {
      const response = await cloudFetch(`/v1/threads/${encodeURIComponent(threadId)}/events${suffix}`, {
        headers: { Accept: 'text/event-stream' },
        signal: controller.signal,
      })
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
          const parsed = parseSseFrame(buffer.slice(0, sep).replace(/\r/g, ''))
          buffer = buffer.slice(sep + 2)
          if (parsed) {
            if (parsed.seq > lastSeq) lastSeq = parsed.seq
            emitCloudEvent(parsed)
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
