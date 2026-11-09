import type { VoiceConnectionResult, VoiceSettings } from '@/lib/api/liveVoice'
import type { SessionEvent, SessionMessages, VoiceMessage } from '@/lib/api/types'
import type { MessageContextInput } from '@/lib/messageContext'
import type { ClaudeVoiceAPI, ClaudeVoiceStart } from '@shared/claudeVoice'
import type { VoiceDiagnostic } from '@/lib/voice/diagnostics'

export type ClaudeVoiceSetup = {
  sessionId: string
  context: string
  signal: AbortSignal
}

export type ClaudeVoicePort = Omit<ClaudeVoiceAPI, 'status' | 'start'> & {
  start(id: string, options: ClaudeVoiceStart, setup: ClaudeVoiceSetup): Promise<string>
}

type VoiceServices = {
  getSettings(signal: AbortSignal): Promise<VoiceSettings>
  connect(sessionId: string, sdp: string, context: string, signal: AbortSignal): Promise<VoiceConnectionResult>
  readSession(sessionId: string, signal: AbortSignal): Promise<SessionMessages>
  subscribeSession(sessionId: string, after: number, onEvent: (event: SessionEvent) => void, onConnection: (connected: boolean) => void): () => void
  sendTask(sessionId: string, message: string, contexts: MessageContextInput[], signal: AbortSignal): Promise<void>
  saveDiagnostics(sessionId: string, records: VoiceDiagnostic[]): Promise<void>
  requestMicrophone(): Promise<MediaStream>
  claude?: ClaudeVoicePort
}

export type VoiceHost = VoiceServices & (
  | { transcript(message: VoiceMessage, final: boolean): void; saveTranscript?: never }
  | { transcript?: undefined; saveTranscript(sessionId: string, messages: VoiceMessage[]): Promise<void> }
)
