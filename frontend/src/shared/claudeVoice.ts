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

export function parseClaudeVoiceCommand(value: unknown): ClaudeVoiceCommand {
  if (!value || typeof value !== 'object' || !('type' in value) || JSON.stringify(value).length > 262144) {
    throw new Error('Invalid Claude voice command.')
  }
  const type = value.type
  if (type !== 'status' && type !== 'start' && type !== 'send' && type !== 'result' && type !== 'close') {
    throw new Error('Invalid Claude voice command.')
  }
  if (type === 'status') {
    return { type }
  }
  if (!('id' in value) || typeof value.id !== 'string' || !/^[a-zA-Z0-9-]{1,128}$/.test(value.id)) {
    throw new Error('Invalid Claude voice call.')
  }
  const id = value.id
  if (type === 'start') {
    if (!('options' in value) || !value.options || typeof value.options !== 'object') {
      throw new Error('Invalid Claude voice connection.')
    }
    const options = value.options
    if (!('sdp' in options) || typeof options.sdp !== 'string' || !options.sdp.startsWith('v=0') || options.sdp.length > 65536 ||
      !('context' in options) || typeof options.context !== 'string' || options.context.length > 131072 ||
      !('voice' in options) || typeof options.voice !== 'string' || options.voice.length > 128) {
      throw new Error('Invalid Claude voice connection.')
    }
    return { type, id, options: { sdp: options.sdp, context: options.context, voice: options.voice } }
  }
  if (type === 'send') {
    if (!('data' in value) || typeof value.data !== 'string') {
      throw new Error('Invalid Claude voice message.')
    }
    return { type, id, data: value.data }
  }
  if (type === 'result') {
    if (!('toolUseId' in value) || typeof value.toolUseId !== 'string' || !('text' in value) || typeof value.text !== 'string' || !('isError' in value) || typeof value.isError !== 'boolean') {
      throw new Error('Invalid Claude voice result.')
    }
    return { type, id, toolUseId: value.toolUseId, text: value.text, isError: value.isError }
  }
  return { type, id }
}
