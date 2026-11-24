import {
  apiFetch as cloudFetch,
  apiSessionRevision,
  onApiSessionChange as onCloudSessionChange,
} from './client'
import type { CloudSessionEvent, CloudThread } from './cloudThreads'
import type { ACPToolCall, ChatMessage, MessageBlock, Session, SessionEvent, SessionMessages } from './types'

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

function isCloudThinkingDeltaEvent(type: string): boolean {
  return type === 'thinking_delta' || type === 'thinking.delta' || type === 'reasoning_delta'
}

function isCloudToolCallEvent(type: string): boolean {
  return type === 'tool_call' || type === 'tool.call'
}

function isCloudToolDeltaEvent(type: string): boolean {
  return type === 'tool_delta' || type === 'tool.delta'
}

function isCloudToolResultEvent(type: string): boolean {
  return type === 'tool_result' || type === 'tool.result'
}

function isCloudToolEvent(type: string): boolean {
  return isCloudToolCallEvent(type) || isCloudToolDeltaEvent(type) || isCloudToolResultEvent(type)
}

function cloudTerminalEventState(type: string): 'idle' | 'failed' | 'cancelled' | null {
  if (type === 'turn_completed' || type === 'turn.completed') return 'idle'
  if (type === 'turn_failed' || type === 'turn.failed') return 'failed'
  if (type === 'turn_interrupted' || type === 'turn.interrupted') return 'cancelled'
  return null
}

function cloudPayloadText(payload: Record<string, unknown>): string {
  for (const key of ['text', 'prompt', 'content', 'delta'] as const) {
    const value = payload[key]
    if (typeof value === 'string' && value) return value
  }
  return ''
}

function normalizeCloudToolStatus(raw: string): string {
  switch (raw.trim().toLowerCase()) {
    case 'completed':
    case 'done':
    case 'ok':
    case 'success':
      return 'completed'
    case 'failed':
    case 'error':
    case 'declined':
    case 'cancelled':
    case 'canceled':
      return 'failed'
    default:
      return 'in_progress'
  }
}

interface CloudToolAccumulator {
  id: string
  tool: string
  title: string
  status: string
  rawInput?: Record<string, unknown>
  streamOut: string
  finalOut: string
  errMsg: string
  startedAt: string
  updatedAt: string
}

function applyCloudToolAccumulator(
  tools: Map<string, CloudToolAccumulator>,
  ev: CloudSessionEvent,
  payload: Record<string, unknown>,
): CloudToolAccumulator | null {
  const callId =
    (typeof payload.call_id === 'string' && payload.call_id) ||
    (typeof payload.id === 'string' && payload.id) ||
    (typeof payload.tool_call_id === 'string' && payload.tool_call_id) ||
    `tool-${ev.seq}`
  let state = tools.get(callId)
  if (!state) {
    state = {
      id: callId,
      tool: '',
      title: '',
      status: 'in_progress',
      streamOut: '',
      finalOut: '',
      errMsg: '',
      startedAt: ev.created_at,
      updatedAt: ev.created_at,
    }
    tools.set(callId, state)
  }
  state.updatedAt = ev.created_at

  if (isCloudToolCallEvent(ev.type)) {
    if (typeof payload.tool === 'string' && payload.tool) state.tool = payload.tool
    if (typeof payload.title === 'string' && payload.title) state.title = payload.title
    state.status = normalizeCloudToolStatus(typeof payload.status === 'string' ? payload.status : '')
    const args = payload.arguments
    if (args && typeof args === 'object' && !Array.isArray(args)) {
      state.rawInput = args as Record<string, unknown>
    } else if (typeof args === 'string' && args.trim()) {
      try {
        const parsed = JSON.parse(args)
        if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
          state.rawInput = parsed as Record<string, unknown>
        }
      } catch {
        // Ignore unparseable string arguments.
      }
    }
    return state
  }
  if (isCloudToolDeltaEvent(ev.type)) {
    const delta = cloudPayloadText(payload)
    if (delta) state.streamOut += delta
    return state
  }
  if (isCloudToolResultEvent(ev.type)) {
    if (typeof payload.tool === 'string' && payload.tool && !state.tool) state.tool = payload.tool
    if (typeof payload.title === 'string' && payload.title && !state.title) state.title = payload.title
    const statusRaw = typeof payload.status === 'string' && payload.status ? payload.status : 'completed'
    state.status = normalizeCloudToolStatus(statusRaw)
    if (typeof payload.output === 'string' && payload.output) state.finalOut = payload.output
    if (typeof payload.error === 'string' && payload.error) {
      state.errMsg = payload.error
      if (state.status === 'in_progress') state.status = 'failed'
    }
    return state
  }
  return null
}

function toACPToolCall(ts: CloudToolAccumulator): ACPToolCall {
  const out = ts.finalOut || ts.streamOut
  const text = ts.errMsg ? (out ? `${out}\n${ts.errMsg}` : ts.errMsg) : out
  return {
    id: ts.id,
    title: ts.title || ts.tool || 'Tool',
    status: ts.status,
    tool_name: ts.tool || undefined,
    raw_input: ts.rawInput,
    ...(text
      ? {
          content: [{ type: 'text', text }],
          ...(ts.tool === 'commandExecution' ? { runtime: { terminal_output: text } } : {}),
          ...(ts.status === 'completed' || ts.status === 'failed'
            ? { raw_output: { output: ts.finalOut, error: ts.errMsg } }
            : {}),
        }
      : {}),
    started_at: ts.startedAt,
    updated_at: ts.updatedAt,
  }
}

function makeCloudThoughtEvent(
  threadId: string,
  ev: CloudSessionEvent,
  payload: Record<string, unknown>,
  runId: number,
  chunk: string,
): SessionEvent {
  const sessionId = ev.thread_id || threadId
  const runStr = String(runId)
  return {
    seq: ev.seq,
    session_id: sessionId,
    type: 'acp_thought',
    projection_key: `acp_thought:${sessionId}:${runStr}`,
    projection_op: 'append',
    acp: {
      id: sessionId,
      slug: sessionId,
      agent: typeof payload.agent === 'string' ? payload.agent : '',
      session_id: sessionId,
      state: 'running',
      thought: chunk,
      text_run_id: runStr,
      last_event_at: ev.created_at,
    },
    at: ev.created_at,
  }
}

function makeCloudToolEvent(
  threadId: string,
  ev: CloudSessionEvent,
  payload: Record<string, unknown>,
  ts: CloudToolAccumulator,
): SessionEvent {
  const sessionId = ev.thread_id || threadId
  const call = toACPToolCall(ts)
  return {
    seq: ev.seq,
    session_id: sessionId,
    type: 'acp_tool',
    projection_key: `acp_tool:${sessionId}:${call.id}`,
    projection_op: 'replace',
    acp: {
      id: sessionId,
      slug: sessionId,
      agent: typeof payload.agent === 'string' ? payload.agent : '',
      session_id: sessionId,
      state: 'running',
      tool_calls: [call],
      last_event_at: ev.created_at,
      last_tool_at: ev.created_at,
    },
    at: ev.created_at,
  }
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

export function projectCloudEventsForSession(
  thread: CloudThread,
  session: Session,
  rawEvents: CloudSessionEvent[],
): SessionMessages {
  const messages: ChatMessage[] = []
  const events: SessionEvent[] = []
  const projectedIndexByKey = new Map<string, number>()
  const tools = new Map<string, CloudToolAccumulator>()
  let latestSeq = 0
  let acpState = session.status === 'running' ? 'running' : 'idle'
  let lastTurnTerminal: 'idle' | 'failed' | 'cancelled' | null = null
  let acpError: string | undefined
  let pendingAssistantText = ''
  let pendingAssistantSeq = 0
  let pendingAssistantAt = ''
  let thoughtRun = 0
  let inThought = false

  const applyProjectedEvent = (ev: SessionEvent) => {
    const key = ev.projection_key
    if (!key) {
      events.push(ev)
      return
    }
    const existingIdx = projectedIndexByKey.get(key)
    if (existingIdx === undefined) {
      projectedIndexByKey.set(key, events.length)
      events.push(ev)
      return
    }
    if (ev.projection_op === 'append' && events[existingIdx]?.acp && ev.acp) {
      events[existingIdx] = {
        ...ev,
        acp: {
          ...events[existingIdx].acp,
          ...ev.acp,
          thought: (events[existingIdx].acp?.thought ?? '') + (ev.acp.thought ?? ''),
        },
      }
      return
    }
    events[existingIdx] = ev
  }

  const flushPendingAssistant = () => {
    tools.clear()
    inThought = false
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
    const text = cloudPayloadText(payload)

    if (isCloudUserEvent(ev.type) && text) {
      flushPendingAssistant()
      lastTurnTerminal = null
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

    if (isCloudThinkingDeltaEvent(ev.type)) {
      if (text) {
        lastTurnTerminal = null
        if (!inThought) {
          thoughtRun += 1
          inThought = true
        }
        applyProjectedEvent(makeCloudThoughtEvent(thread.id, ev, payload, thoughtRun, text))
      }
      continue
    }

    if (isCloudToolEvent(ev.type)) {
      inThought = false
      lastTurnTerminal = null
      const ts = applyCloudToolAccumulator(tools, ev, payload)
      if (ts) {
        applyProjectedEvent(makeCloudToolEvent(thread.id, ev, payload, ts))
      }
      continue
    }

    if (isCloudAssistantDeltaEvent(ev.type)) {
      inThought = false
      if (text) {
        lastTurnTerminal = null
        pendingAssistantText += text
        pendingAssistantSeq = ev.seq
        pendingAssistantAt = ev.created_at
      }
      continue
    }

    if (isCloudAssistantMessageEvent(ev.type) && text) {
      inThought = false
      lastTurnTerminal = null
      pendingAssistantText = ''
      tools.clear()
      messages.push({
        seq: ev.seq,
        role: 'assistant',
        content: text,
        blocks: [{ type: 'text', text }],
        created_at: ev.created_at,
      })
      continue
    }

    inThought = false
    const terminalState = cloudTerminalEventState(ev.type)
    if (terminalState) {
      flushPendingAssistant()
      lastTurnTerminal = terminalState
      acpState = terminalState
      acpError = typeof payload.error === 'string' && payload.error.trim() ? payload.error.trim() : undefined
    }

    applyProjectedEvent(cloudEventToSessionEvent(thread.id, ev))
  }

  if (lastTurnTerminal) {
    acpState = lastTurnTerminal
    if (session.status === 'running') {
      session.status = lastTurnTerminal === 'failed' ? 'error' : lastTurnTerminal === 'cancelled' ? 'interrupted' : 'idle'
    }
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
  let thoughtRun = 0
  let inThought = false
  const tools = new Map<string, CloudToolAccumulator>()
  const revision = apiSessionRevision()

  function emitCloudEvent(parsed: CloudSessionEvent): void {
    const payload = parsed.payload && typeof parsed.payload === 'object' ? parsed.payload : {}
    if (isCloudUserEvent(parsed.type)) {
      streamedAssistant = ''
      inThought = false
      tools.clear()
    } else if (isCloudThinkingDeltaEvent(parsed.type)) {
      const chunk = cloudPayloadText(payload)
      if (!chunk) return
      if (!inThought) {
        thoughtRun += 1
        inThought = true
      }
      onEvent(makeCloudThoughtEvent(threadId, parsed, payload, thoughtRun, chunk))
      return
    } else if (isCloudToolEvent(parsed.type)) {
      inThought = false
      const ts = applyCloudToolAccumulator(tools, parsed, payload)
      if (ts) {
        onEvent(makeCloudToolEvent(threadId, parsed, payload, ts))
      }
      return
    } else if (isCloudAssistantDeltaEvent(parsed.type)) {
      inThought = false
      const chunk = cloudPayloadText(payload)
      streamedAssistant += chunk
      onEvent(cloudEventToSessionEvent(threadId, {
        ...parsed,
        payload: { ...payload, text: streamedAssistant },
      }))
      return
    } else if (cloudTerminalEventState(parsed.type)) {
      streamedAssistant = ''
      inThought = false
      tools.clear()
    } else {
      inThought = false
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
