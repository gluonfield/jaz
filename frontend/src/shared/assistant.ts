import type { BotAvatar } from './bots'

export type AssistantWakeMode = 'voice' | 'dictate'

export type AssistantAction =
  | { mode: 'default' | 'voice' | 'dictate' }
  | { mode: 'send'; text: string }

export type QuickDictatePhase =
  | 'starting'
  | 'downloading'
  | 'listening'
  | 'transcribing'
  | 'sending'
  | 'sent'
  | 'error'

export interface QuickDictateState {
  phase: QuickDictatePhase
  botName: string
  botId?: string
  avatar?: BotAvatar
  text: string
  level: number
  error?: string
  notch: { width: number; height: number } | null
}

export type QuickDictatePublishState = Omit<QuickDictateState, 'notch'>

export type QuickDictateCommand =
  | { type: 'send' }
  | { type: 'cancel' }
  | { type: 'open' }
  | { type: 'hover'; inside: boolean }

export const ASSISTANT_CHANNEL = 'jaz:assistant:'

export interface AssistantAPI {
  onTrigger: (handler: (action: AssistantAction) => void) => () => void
  trigger: (action: AssistantAction) => void
  focusMain: () => void
  publishQuickDictate: (state: QuickDictatePublishState | null) => void
  onQuickDictateCommand: (handler: (command: QuickDictateCommand) => void) => () => void
  subscribeQuickDictate: (handler: (state: QuickDictateState | null) => void) => () => void
  sendQuickDictateCommand: (command: QuickDictateCommand) => void
}

export const ASSISTANT_URLS = {
  default: 'jaz://assistant',
  voice: 'jaz://assistant?mode=voice',
  dictate: 'jaz://assistant?mode=dictate',
  sendExample: 'jaz://assistant?text=',
} as const

export function parseAssistantDeepLink(rawUrl: string): AssistantAction | null {
  try {
    const parsed = new URL(rawUrl.trim())
    if (parsed.protocol !== 'jaz:' || parsed.hostname.toLowerCase() !== 'assistant') {
      return null
    }
    const modeParam = parsed.searchParams.get('mode')?.trim().toLowerCase() ?? ''
    const voiceParam = parsed.searchParams.get('voice')?.trim().toLowerCase() ?? ''
    const dictateParam = parsed.searchParams.get('dictate')?.trim().toLowerCase() ?? ''
    const textParam = parsed.searchParams.get('text')?.trim() ?? ''

    if (modeParam === 'voice' || voiceParam === '1' || voiceParam === 'true') {
      return { mode: 'voice' }
    }
    if (modeParam === 'dictate' || dictateParam === '1' || dictateParam === 'true') {
      return { mode: 'dictate' }
    }
    if (modeParam === 'send') {
      return textParam ? { mode: 'send', text: textParam } : null
    }
    if (textParam) {
      return { mode: 'send', text: textParam }
    }
    if (!modeParam || modeParam === 'default') {
      return { mode: 'default' }
    }
    return null
  } catch {
    return null
  }
}

export function extractAssistantDeepLinkArg(argv: readonly string[]): AssistantAction | null {
  for (const arg of argv) {
    const action = parseAssistantDeepLink(arg)
    if (action) return action
  }
  return null
}

export function isAssistantWakeMode(value: unknown): value is AssistantWakeMode {
  return value === 'voice' || value === 'dictate'
}

export function parseAssistantAction(value: unknown): AssistantAction | null {
  if (!value || typeof value !== 'object') return null
  const input = value as Record<string, unknown>
  if (input.mode === 'default' || input.mode === 'voice' || input.mode === 'dictate') {
    return { mode: input.mode }
  }
  if (input.mode === 'send' && typeof input.text === 'string') {
    const text = input.text.trim()
    return text ? { mode: 'send', text } : null
  }
  return null
}
