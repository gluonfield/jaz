import { connectBrowser } from '@/lib/browserConnection'
import type { ClaudeVoiceAPI, ClaudeVoiceCommand } from '@shared/claudeVoice'

export function connectClaudeVoiceHost(api: ClaudeVoiceAPI): () => void {
  const calls = new Set<string>()
  let emit: (message: unknown) => void = () => {}
  const cancel = () => {
    for (const id of calls) {
      void api.close(id).catch(() => {})
    }
    calls.clear()
  }
  const unsubscribe = api.onEvent((id, data) => {
    if (calls.has(id)) {
      emit({ method: 'Claude.voiceEvent', params: { id, data } })
    }
  })
  const stop = connectBrowser('/v1/voice/desktop', async (request, send) => {
    if (request.method !== 'Claude.voice' || !request.params) {
      throw new Error('Invalid Claude voice relay request.')
    }
    const command = request.params as ClaudeVoiceCommand
    emit = send
    if (command.type === 'status') {
      return api.status()
    }
    if (typeof command.id !== 'string' || !command.id) {
      throw new Error('Invalid Claude voice call.')
    }
    if (command.type === 'start') {
      calls.add(command.id)
      try {
        return await api.start(command.id, command.options)
      } catch (error) {
        calls.delete(command.id)
        void api.close(command.id).catch(() => {})
        throw error
      }
    }
    if (!calls.has(command.id)) {
      if (command.type === 'close') {
        return
      }
      throw new Error('Claude voice call is not owned by this connection.')
    }
    switch (command.type) {
      case 'send':
        return api.send(command.id, command.data)
      case 'result':
        return api.result(command.id, command.toolUseId, command.text, command.isError)
      case 'close':
        calls.delete(command.id)
        return api.close(command.id)
      default:
        throw new Error('Unsupported Claude voice relay command.')
    }
  }, cancel)
  return () => {
    unsubscribe()
    stop()
  }
}
