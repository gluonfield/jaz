import { ipcRenderer } from 'electron'
import { CLAUDE_VOICE_CHANNEL, type ClaudeVoiceAPI } from '@shared/claudeVoice'

export const claudeVoice: ClaudeVoiceAPI = {
  status: () => ipcRenderer.invoke(CLAUDE_VOICE_CHANNEL, { type: 'status' }),
  start: (id, options) => ipcRenderer.invoke(CLAUDE_VOICE_CHANNEL, { type: 'start', id, options }),
  send: (id, data) => ipcRenderer.invoke(CLAUDE_VOICE_CHANNEL, { type: 'send', id, data }),
  result: (id, toolUseId, text, isError) => ipcRenderer.invoke(CLAUDE_VOICE_CHANNEL, { type: 'result', id, toolUseId, text, isError }),
  close: (id) => ipcRenderer.invoke(CLAUDE_VOICE_CHANNEL, { type: 'close', id }),
  onEvent: (handler) => {
    const listener = (_event: unknown, id: string, data: string) => handler(id, data)
    ipcRenderer.on(CLAUDE_VOICE_CHANNEL + ':event', listener)
    return () => ipcRenderer.removeListener(CLAUDE_VOICE_CHANNEL + ':event', listener)
  },
}
