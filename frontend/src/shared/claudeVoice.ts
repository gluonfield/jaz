export const CLAUDE_VOICE_CHANNEL = 'jaz:claude-voice'
export const CLAUDE_VOICE_ARGUMENT = '--jaz-claude-voice'

export type ClaudeVoiceStatus = {
  available: boolean
  reason?: string
}

export type ClaudeVoiceStart = {
  sdp: string
  voice: string
  context: string
}

export type ClaudeVoiceCommand =
  | { type: 'status' }
  | { type: 'start'; id: string; options: ClaudeVoiceStart }
  | { type: 'send'; id: string; data: string }
  | { type: 'result'; id: string; toolUseId: string; text: string; isError: boolean }
  | { type: 'close'; id: string }

export type ClaudeVoiceReply = {
  requestId: number
  value?: ClaudeVoiceStatus | string
  error?: string
}

export interface ClaudeVoiceAPI {
  status(): Promise<ClaudeVoiceStatus>
  start(id: string, options: ClaudeVoiceStart): Promise<string>
  send(id: string, data: string): Promise<void>
  result(id: string, toolUseId: string, text: string, isError: boolean): Promise<void>
  close(id: string): Promise<void>
  onEvent(handler: (id: string, data: string) => void): () => void
}
