import { queryOptions } from '@tanstack/react-query'
import type { VoiceMessage } from '@/lib/api/types'
import type { MessageContextInput } from '@/lib/messageContext'
import { get, post, put } from '@/lib/api/client'
import { keys } from '@/lib/query/keys'

export type VoiceProvider = 'openai' | 'openai-api-key'

export interface VoiceSettings {
  agent: 'openai'
  provider: VoiceProvider
  voice: string
  voices: string[]
  providers: { id: VoiceProvider; label: string; available: boolean; reason?: string }[]
}

export const voiceSettingsQuery = queryOptions({
  queryKey: keys.voiceSettings,
  queryFn: () => get<VoiceSettings>('/v1/settings/voice'),
})

export const voiceAvailableQuery = queryOptions({
  ...voiceSettingsQuery,
  select: (settings) => settings.providers.some((provider) => provider.id === settings.provider && provider.available),
})

export function updateVoiceSettings(settings: Pick<VoiceSettings, 'provider' | 'voice'>): Promise<VoiceSettings> {
  return put('/v1/settings/voice', { agent: 'openai', ...settings })
}

export function connectVoice(sessionId: string, sdp: string, context: string, signal: AbortSignal): Promise<{
  sdp: string
  provider: VoiceProvider
  model: string
}> {
  return post('/v1/voice/connect', { session_id: sessionId, sdp, context }, signal)
}

export function sendVoiceTask(sessionId: string, message: string, contexts: MessageContextInput[], signal: AbortSignal): Promise<void> {
  return post(`/v1/sessions/${sessionId}/agent/input`, { message, contexts }, signal)
}

export function saveVoiceTranscript(sessionId: string, messages: VoiceMessage[]): Promise<void> {
  return post(`/v1/sessions/${sessionId}/voice/transcript`, messages)
}
