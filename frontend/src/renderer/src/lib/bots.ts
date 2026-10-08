import type { ACPPermission, Attachment, Bot, BotActivityEvent, BotAvatar, BotColor, BotShape, ChatMessage, MCPAppEvent, MCPEntrypoint, SessionEvent } from '@/lib/api/types'
import { messageText } from '@/lib/messageText'
import { isPresentedApp } from '@/lib/mcpApps'
import { hasPermissionSurface } from '@/lib/sessionPermissions'
import { type SpawnedThreadView, threadRunning } from '@/lib/spawnedThreads'

export const BOT_SHAPES: BotShape[] = ['circle', 'blob', 'squircle', 'pill', 'triangle', 'hex', 'cloud', 'drop']

// Fills bright enough to carry dark eyes on both themes.
export const BOT_COLORS: Record<BotColor, string> = {
  white: '#f4f4f5',
  brown: '#c08b5c',
  red: '#f45d5d',
  orange: '#f98c46',
  amber: '#f6c443',
  green: '#5bcb72',
  teal: '#37c6b4',
  blue: '#58a0f8',
  purple: '#a682f6',
  pink: '#f47fbf',
  gray: '#a3a3ad',
}

// A bot's colour drawn as text or marks: mixed toward ink, so white and the
// pale colours stay readable in either theme.
export function botInk(color: BotColor): string {
  return `color-mix(in oklab, ${BOT_COLORS[color]} 65%, var(--color-ink))`
}

const pick = <T>(items: T[]): T => items[Math.floor(Math.random() * items.length)]

// A random face skips the neutral white and gray.
const VIVID = (Object.keys(BOT_COLORS) as BotColor[]).filter((color) => color !== 'white' && color !== 'gray')

export function randomAvatar(): BotAvatar {
  return { shape: pick(BOT_SHAPES), color: pick(VIVID) }
}

const LAST_BOT_KEY = 'jaz.lastBot'

// The Bots tab returns to the chat the user left, as Chat returns to its thread.
export const lastBotId = () => localStorage.getItem(LAST_BOT_KEY) ?? undefined
export const rememberBot = (id: string) => localStorage.setItem(LAST_BOT_KEY, id)

const TARGET_PREFIX = 'bot:'

export const botTarget = (id: string) => `${TARGET_PREFIX}${id}`

export function botIdFromTarget(target: string): string | undefined {
  return target.startsWith(TARGET_PREFIX) ? target.slice(TARGET_PREFIX.length) : undefined
}

export function pinOrder(bots: Bot[]): string[] {
  return bots
    .filter((bot) => bot.pinned)
    .toSorted((a, b) => (a.pinned ?? 0) - (b.pinned ?? 0) || a.name.localeCompare(b.name))
    .map((bot) => bot.id)
}

// Pinned tiles in pin order and the list below by activity. Mid-drag every
// bot shows where it would land, and the dragged bot is also left hidden where
// it started once its preview moves to the other section, because the element
// a drag starts from must stay mounted for it to end.
export function dragSections(bots: Bot[], pins: string[], drag?: { id: string; pins: string[] }) {
  const shown = drag?.pins ?? pins
  const tiles = shown.flatMap((id) => bots.find((bot) => bot.id === id) ?? []).map((bot) => ({ bot, hidden: false }))
  const rows = bots
    .filter((bot) => !shown.includes(bot.id))
    .toSorted((a, b) => Date.parse(b.updated_at) - Date.parse(a.updated_at))
    .map((bot) => ({ bot, hidden: false }))
  const source = drag && bots.find((bot) => bot.id === drag.id)
  if (source && pins.includes(source.id) !== drag.pins.includes(source.id)) {
    const origin = pins.includes(source.id) ? tiles : rows
    origin.push({ bot: source, hidden: true })
  }
  return { tiles, rows }
}

// The pins with id moved beside target, after it when `after`; with no
// target, id joins the end unless it is already pinned.
export function placePin(pins: string[], id: string, target: string | undefined, after: boolean): string[] {
  if (target === id || (!target && pins.includes(id))) return pins
  const others = pins.filter((other) => other !== id)
  const at = target ? others.indexOf(target) + Number(after) : others.length
  return [...others.slice(0, at), id, ...others.slice(at)]
}

// A group wears its first two members' faces, falling back to its own.
export function botAvatars(bot: Bot, bots: Bot[]): BotAvatar[] {
  if (bot.kind === 'bot') return [bot.avatar]
  const members = (bot.members ?? [])
    .flatMap((id) => bots.find((member) => member.id === id)?.avatar ?? [])
    .slice(0, 2)
  return members.length ? members : [bot.avatar]
}

export type ChatEntry =
  | { kind: 'user'; key: string; at: string; text: string; attachments?: Attachment[]; attachmentSessionId?: string }
  | { kind: 'bot'; key: string; at: string; botId?: string; name: string; text: string }
  | { kind: 'activity'; key: string; at: string; event: SessionEvent }
  | { kind: 'app'; key: string; at: string; app: MCPAppEvent }
  | { kind: 'question'; key: string; at: string; event: SessionEvent; answer?: ACPPermission }

type ChatTurn = {
  at: string
  user: boolean
  activity?: BotActivityEvent
}

export type BotWork = { doing?: string }

// A bot's chat, read from its thread in one pass: what people typed, what bots
// sent with send_message, presented apps, questions with their answers, and
// activity rows. Lookups and work for other bots stay private.
// A bot's turn opens when the person speaks to it or an activity, such as a
// routine or another bot's message, starts work; messaging another bot does not.
export function opensBotTurn(event: SessionEvent): boolean {
  return event.room_message?.speaker === 'user' || (!!event.bot_activity && event.bot_activity.kind !== 'message_sent')
}

export function botChat(
  messages: ChatMessage[],
  events: SessionEvent[],
  self: { id: string; name: string },
  entrypoints: MCPEntrypoint[],
): { entries: ChatEntry[]; work: BotWork; waiting: boolean } {
  const items = [
    ...messages.flatMap((message) =>
      message.role === 'user' ? [{ at: message.created_at, message, event: undefined }] : [],
    ),
    ...events.map((event) => ({ at: event.at, message: undefined, event })),
  ].sort((a, b) => Date.parse(a.at) - Date.parse(b.at))
  const entries: ChatEntry[] = []
  const questions = new Map<string, Extract<ChatEntry, { kind: 'question' }>>()
  const answers = new Map<string, ACPPermission>()
  let turn: ChatTurn | undefined
  for (const { at, message, event } of items) {
    if (message) {
      entries.push({
        kind: 'user', key: `message:${message.seq}:${at}`, at, text: messageText(message),
        attachments: message.blocks?.filter((block) => block.type === 'attachment'),
        attachmentSessionId: self.id,
      })
      turn = { at, user: true }
      continue
    }
    const key = `${event.session_id}:${event.seq ?? at}`
    const room = event.room_message
    const activity = event.bot_activity
    if (room) {
      if (room.speaker === 'user') entries.push({ kind: 'user', key, at, text: room.text, attachments: room.attachments, attachmentSessionId: self.id })
      else entries.push({ kind: 'bot', key, at, botId: room.bot_id, name: room.name, text: room.text })
    } else if (activity) {
      // Messaging another bot happens within a turn; anything else starts one.
      const opens = activity.kind !== 'message_sent'
      if (activity.kind === 'message_sent' || activity.kind === 'message_received' || activity.kind === 'unreachable') entries.push({ kind: 'activity', key, at, event })
      if (opens) turn = { at, user: false, activity }
    } else if (event.type === 'mcp_app' && event.mcp_app && (event.mcp_app.presented || turn?.user || turn?.activity?.kind === 'routine') && isPresentedApp(event.mcp_app, entrypoints)) {
      entries.push({ kind: 'app', key, at, app: event.mcp_app })
    } else if (event.type === 'permission_request' && event.permission && hasPermissionSurface(event.permission)) {
      // A question asked again replaces its card; the latest one is answered.
      const asked = questions.get(event.permission.id)
      if (asked) {
        asked.event = event
      } else {
        const entry = { kind: 'question' as const, key: `question:${event.permission.id}`, at, event }
        questions.set(event.permission.id, entry)
        entries.push(entry)
      }
    } else if (event.type === 'permission_response' && event.permission) {
      answers.set(event.permission.id, event.permission)
    } else if (event.loop_created || event.type === 'agent_switch') {
      entries.push({ kind: 'activity', key, at, event })
    }
  }
  for (const [id, entry] of questions) entry.answer = answers.get(id)
  // A bot held on the user's answer to its question is waiting, not working.
  const waiting = [...questions.values()].some((entry) => !entry.answer)
  return { entries, work: { doing: busyWith(turn?.activity) }, waiting }
}

// What a bot's running subtasks have it doing, said as one person doing
// several things at once.
export function subtasksDoing(threads: SpawnedThreadView[]): string | undefined {
  const titles = threads.filter(threadRunning).map((thread) => thread.title || thread.slug)
  return titles.length ? `working on ${titles.join(', ')}` : undefined
}

function busyWith(activity?: BotActivityEvent): string | undefined {
  switch (activity?.kind) {
    case 'message_received':
      return `working on ${activity.label}'s message`
    case 'routine':
      return `running ${activity.label}`
  }
  return undefined
}
