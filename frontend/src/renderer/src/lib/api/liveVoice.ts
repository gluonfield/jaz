import { queryOptions } from '@tanstack/react-query'
import type { VoiceMessage } from '@/lib/api/types'
import type { MessageContextInput } from '@/lib/messageContext'
import { get, post, put } from '@/lib/api/client'

export type VoiceProvider = 'openai' | 'openai-api-key'

export interface VoiceSettings {
  agent: 'openai'
  provider: VoiceProvider
  voice: string
  voices: string[]
  providers: { id: VoiceProvider; label: string; available: boolean; reason?: string }[]
}

export const voiceSettingsQuery = queryOptions({
  queryKey: ['settings', 'voice'],
  queryFn: () => get<VoiceSettings>('/v1/settings/voice'),
})

export function updateVoiceSettings(settings: Pick<VoiceSettings, 'provider' | 'voice'>): Promise<VoiceSettings> {
  return put('/v1/settings/voice', { agent: 'openai', ...settings })
}

export function connectVoice(sdp: string, context: string, signal: AbortSignal): Promise<{
  sdp: string
  provider: VoiceProvider
  model: string
}> {
  return post('/v1/voice/connect', { sdp, context }, signal)
}

export function sendVoiceTask(sessionId: string, message: string, contexts: MessageContextInput[], signal: AbortSignal): Promise<void> {
  return post(`/v1/sessions/${sessionId}/agent/input`, { message, contexts }, signal)
}

export function saveVoiceTranscript(sessionId: string, messages: VoiceMessage[]): Promise<void> {
  return post(`/v1/sessions/${sessionId}/voice/transcript`, messages)
}
