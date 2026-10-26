import { queryOptions } from '@tanstack/react-query'
import type { VoiceMessage } from '@/lib/api/types'
import type { MessageContextInput } from '@/lib/messageContext'
import type { VoiceDiagnostic } from '@/lib/voice/diagnostics'
import { get, post, put } from '@/lib/api/client'
import { keys } from '@/lib/query/keys'

export type VoiceProvider = 'openai' | 'openai-api-key' | 'claude'

export interface VoiceSettings {
  agent: 'openai' | 'claude'
  provider: VoiceProvider
  voice: string
  voices: string[]
  providers: { id: VoiceProvider; label: string; available: boolean; reason?: string }[]
}

export async function getVoiceSettings(): Promise<VoiceSettings> {
  const settings = await get<VoiceSettings>('/v1/settings/voice')
  if (settings.provider !== 'claude' || !window.jaz?.claudeVoice) {
    return settings
  }
  const local = await window.jaz.claudeVoice.status().catch((error: Error) => ({ available: false, reason: error.message }))
  return {
    ...settings,
    providers: settings.providers.map((provider) => provider.id === 'claude'
      ? { ...provider, available: local.available, reason: local.reason }
      : provider),
  }
}

export const voiceSettingsQuery = queryOptions({
  queryKey: keys.voiceSettings,
  queryFn: getVoiceSettings,
})

export const voiceAvailableQuery = queryOptions({
  ...voiceSettingsQuery,
  select: (settings) => settings.providers.some((provider) => provider.id === settings.provider && provider.available),
})

export function updateVoiceSettings(settings: Pick<VoiceSettings, 'provider' | 'voice'>): Promise<VoiceSettings> {
  return put('/v1/settings/voice', { agent: settings.provider === 'claude' ? 'claude' : 'openai', ...settings }).then(() => getVoiceSettings())
}

export function connectVoice(sessionId: string, sdp: string, context: string, signal: AbortSignal): Promise<
  | { provider: 'claude'; instructions: string }
  | { provider: 'openai' | 'openai-api-key'; model: string; sdp: string }
> {
  return post('/v1/voice/connect', { session_id: sessionId, sdp, context }, signal)
}

export function sendVoiceTask(sessionId: string, message: string, contexts: MessageContextInput[], signal: AbortSignal): Promise<void> {
  return post(`/v1/sessions/${sessionId}/agent/input`, { message, contexts }, signal)
}

export function saveVoiceTranscript(sessionId: string, messages: VoiceMessage[]): Promise<void> {
  return post(`/v1/sessions/${sessionId}/voice/transcript`, messages)
}

export function saveVoiceDiagnostics(sessionId: string, records: VoiceDiagnostic[]): Promise<void> {
  return post(`/v1/sessions/${sessionId}/voice/diagnostics`, records)
}
