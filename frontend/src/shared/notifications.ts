import type { BotAvatar, BotColor, BotShape } from './bots'

// What the backend's notification classifier decided: a task met (done), a
// final reply (reply), a question (needs_you) or an error stop (stopped).
export type AlertState = 'done' | 'reply' | 'needs_you' | 'stopped'
export type AlertKind = 'chat' | 'bot' | 'group'

export interface Alert {
  sessionId: string
  key: string
  state: AlertState
  kind: AlertKind
  // The bot's or group's name, also on a question from a thread working for
  // it; empty for a plain thread, which is Jaz's.
  name: string
  avatars: BotAvatar[]
  // The thread title or the completed task; empty for a bot's new reply.
  title: string
  body: string
  requestId: string
  at: number
}

export type NotificationMode = 'off' | 'center' | 'notch' | 'notch_ears'

// What the island still needs, read without asking macOS anything: nothing
// (ready), notifications allowed, Focus shared once asked, Focus allowed after
// a refusal, or Focus readable at all (unavailable).
export type NotchAccess = 'ready' | 'notifications' | 'ask' | 'denied' | 'unavailable'

export type NotificationConfig =
  | { mode: 'off' }
  | { mode: Exclude<NotificationMode, 'off'>; baseUrl: string; token: string }

const MODES: readonly NotificationMode[] = ['off', 'center', 'notch', 'notch_ears']
const STATES: readonly AlertState[] = ['done', 'reply', 'needs_you', 'stopped']
const KINDS: readonly AlertKind[] = ['chat', 'bot', 'group']
const SHAPES: readonly BotShape[] = ['circle', 'blob', 'squircle', 'pill', 'triangle', 'hex', 'cloud', 'drop']
const COLORS: readonly BotColor[] = ['white', 'brown', 'red', 'orange', 'amber', 'green', 'teal', 'blue', 'purple', 'pink', 'gray']

export function isNotificationMode(value: unknown): value is NotificationMode {
  return MODES.includes(value as NotificationMode)
}

export function parseNotificationConfig(value: unknown): NotificationConfig | null {
  if (!value || typeof value !== 'object') return null
  const input = value as Record<string, unknown>
  if (input.mode === 'off') return { mode: 'off' }
  if (!isNotificationMode(input.mode) || typeof input.baseUrl !== 'string' || typeof input.token !== 'string') return null
  try {
    const url = new URL(input.baseUrl.trim())
    if (url.protocol !== 'http:' && url.protocol !== 'https:') return null
    return { mode: input.mode, baseUrl: url.origin, token: input.token.trim() }
  } catch {
    return null
  }
}

const text = (value: unknown) => (typeof value === 'string' ? value.trim() : '')

export function parseAlert(value: unknown): Alert | null {
  if (!value || typeof value !== 'object') return null
  const input = value as Record<string, unknown>
  const at = Date.parse(text(input.at))
  const state = input.state as AlertState
  const kind = input.kind as AlertKind
  const sessionId = text(input.session_id)
  const key = text(input.key)
  if (!sessionId || !key || Number.isNaN(at) || !STATES.includes(state) || !KINDS.includes(kind)) return null
  const avatars = (Array.isArray(input.avatars) ? input.avatars : []).filter(
    (avatar): avatar is BotAvatar => SHAPES.includes(avatar?.shape) && COLORS.includes(avatar?.color),
  )
  return {
    sessionId,
    key,
    state,
    kind,
    name: text(input.name),
    avatars: avatars.map(({ shape, color }) => ({ shape, color })),
    title: text(input.title),
    body: text(input.body),
    requestId: text(input.request_id),
    at,
  }
}

export function alertPath(alert: Pick<Alert, 'sessionId' | 'kind'>): string {
  return `/${alert.kind === 'chat' ? 'sessions' : 'bots'}/${encodeURIComponent(alert.sessionId)}`
}

// The state in a word; a reply has none, since the alert arriving says it.
export function alertLabel(alert: Pick<Alert, 'state'>): string {
  switch (alert.state) {
    case 'done':
      return 'Done'
    case 'needs_you':
      return 'Needs you'
    case 'stopped':
      return 'Stopped'
    case 'reply':
      return ''
  }
}

// Who the alert is from: the bot or group, or Jaz for a plain thread.
export function alertSender(alert: Pick<Alert, 'name'>): string {
  return alert.name || 'Jaz'
}

// The island's state, sent from the main process to its window. `notch` is the
// camera housing it hugs, in points; null draws a floating capsule instead.
export interface IslandState {
  rows: Alert[]
  more: number
  ears: boolean
  notch: { width: number; height: number } | null
}

export type IslandCommand =
  | { type: 'open'; sessionId: string }
  | { type: 'dismiss'; sessionId: string }
  | { type: 'more' }
  | { type: 'hover'; inside: boolean }

export const ISLAND_CHANNEL = 'jaz:island:'

export interface IslandAPI {
  onState: (listener: (state: IslandState) => void) => () => void
  command: (command: IslandCommand) => void
}

// Window bounds in points. Each camera side reserves twice the ear width
// for content-sized ears, including the renderer's 192 pt inline title.
export const ISLAND_EAR_WIDTH = 140
export const ISLAND_CAPSULE_WIDTH = 420
export const ISLAND_HEIGHT = 280
export const ISLAND_HOLD_MS = 10_000
// How long the island takes to close; its window hides once it has.
export const ISLAND_RETRACT_MS = 360
