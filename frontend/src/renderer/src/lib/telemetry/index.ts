const TELEMETRY_ENABLED_KEY = 'jaz.telemetry.enabled'
const TELEMETRY_ID_KEY = 'jaz.telemetry.id'
const TELEMETRY_EVENT = 'jaz:telemetry'
const DEFAULT_HOST = 'https://us.i.posthog.com'

type TelemetryEvent =
  | 'message_sent'
  | 'thread_created'
  | 'loop_created'
  | 'loop_run_started'

type TelemetryProperty = string | number | boolean | null | undefined
type TelemetryProperties = Record<string, TelemetryProperty>

interface MessageSent {
  queued: boolean
  voice?: boolean
  planRequested: boolean
  attachmentCount: number
}

interface ThreadCreated {
  worktree: boolean
  hasDirectory: boolean
  hasModelOverride: boolean
  hasProviderOverride: boolean
  hasReasoningEffort: boolean
}

interface LoopCreated {
  runAfterCreate: boolean
  scheduleKind: string
  status: string
  hasDirectory: boolean
  boardCount: number
  hasModelOverride: boolean
  hasProviderOverride: boolean
  hasReasoningEffort: boolean
}

export function telemetryEnabled(): boolean {
  if (typeof window === 'undefined') return false
  try {
    return window.localStorage.getItem(TELEMETRY_ENABLED_KEY) !== 'false'
  } catch {
    return false
  }
}

export function setTelemetryEnabled(enabled: boolean) {
  if (typeof window === 'undefined') return
  try {
    if (enabled) window.localStorage.removeItem(TELEMETRY_ENABLED_KEY)
    else {
      window.localStorage.setItem(TELEMETRY_ENABLED_KEY, 'false')
      window.localStorage.removeItem(TELEMETRY_ID_KEY)
    }
  } catch {
    return
  }
  window.dispatchEvent(new Event(TELEMETRY_EVENT))
}

export const telemetry = {
  loopCreated,
  loopRunStarted,
  messageSent,
  threadCreated,
  enabled: telemetryEnabled,
  setEnabled: setTelemetryEnabled,
  subscribe,
}

function messageSent(input: MessageSent) {
  capture('message_sent', {
    queued: input.queued,
    voice: input.voice,
    plan_requested: input.planRequested,
    attachment_count: input.attachmentCount,
  })
}

function threadCreated(input: ThreadCreated) {
  capture('thread_created', {
    worktree: input.worktree,
    has_directory: input.hasDirectory,
    has_model_override: input.hasModelOverride,
    has_provider_override: input.hasProviderOverride,
    has_reasoning_effort: input.hasReasoningEffort,
  })
}

function loopCreated(input: LoopCreated) {
  capture('loop_created', {
    run_after_create: input.runAfterCreate,
    schedule_kind: input.scheduleKind,
    status: input.status,
    has_directory: input.hasDirectory,
    board_count: input.boardCount,
    has_model_override: input.hasModelOverride,
    has_provider_override: input.hasProviderOverride,
    has_reasoning_effort: input.hasReasoningEffort,
  })
}

function loopRunStarted() {
  capture('loop_run_started')
}

function capture(event: TelemetryEvent, properties: TelemetryProperties = {}) {
  const token = import.meta.env.VITE_POSTHOG_TOKEN?.trim()
  if (!token || !telemetryEnabled()) return

  const distinctID = telemetryID()
  if (!distinctID) return

  const payload = {
    api_key: token,
    event,
    distinct_id: distinctID,
    properties: compactProperties({
      ...properties,
      $process_person_profile: false,
      app: 'jaz',
      surface: window.jaz?.windowKind || 'main',
      telemetry_version: 1,
    }),
    timestamp: new Date().toISOString(),
  }

  void send(payload)
}

function subscribe(callback: () => void): () => void {
  const listener = () => callback()
  window.addEventListener(TELEMETRY_EVENT, listener)
  window.addEventListener('storage', listener)
  return () => {
    window.removeEventListener(TELEMETRY_EVENT, listener)
    window.removeEventListener('storage', listener)
  }
}

function telemetryID(): string {
  try {
    const stored = window.localStorage.getItem(TELEMETRY_ID_KEY)
    if (stored) return stored
    const id = window.crypto?.randomUUID?.() ?? randomID()
    window.localStorage.setItem(TELEMETRY_ID_KEY, id)
    return id
  } catch {
    return ''
  }
}

function randomID(): string {
  const bytes = new Uint8Array(16)
  if (!window.crypto?.getRandomValues) return `local-${Date.now().toString(16)}-${Math.random().toString(16).slice(2)}`
  window.crypto.getRandomValues(bytes)
  return Array.from(bytes, (byte) => byte.toString(16).padStart(2, '0')).join('')
}

function compactProperties(properties: TelemetryProperties): Record<string, string | number | boolean | null> {
  return Object.fromEntries(
    Object.entries(properties).filter(([, value]) => value !== undefined),
  ) as Record<string, string | number | boolean | null>
}

async function send(payload: object): Promise<void> {
  const host = import.meta.env.VITE_POSTHOG_HOST?.trim() || DEFAULT_HOST
  try {
    await fetch(`${host.replace(/\/+$/, '')}/i/v0/e/`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
      keepalive: true,
    })
  } catch {
    return
  }
}
