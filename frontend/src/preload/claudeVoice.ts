import { ipcRenderer } from 'electron'
import { CLAUDE_VOICE_CHANNEL, type ClaudeVoiceCommand, type ClaudeVoiceReply, type ClaudeVoiceStatus } from '@shared/claudeVoice'

type ModelCatalog = { model_selector_config: { id: string; models: { id: string; name: string; disabled?: boolean }[] }[] }
type Call = { socket: WebSocket; organization: string; conversation: string; keepAlive: ReturnType<typeof setInterval> }

export function installClaudeVoice(): void {
  const calls = new Map<string, Call>()
  const starting = new Map<string, AbortController>()

  async function request(path: string, body?: object, signal?: AbortSignal): Promise<Response> {
    const response = await fetch('/api/' + path, {
      credentials: 'same-origin',
      signal,
      ...(body ? { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) } : {}),
    })
    if (!response.ok) {
      throw new Error(response.status === 401 || response.status === 403
        ? 'Sign in to Claude in the Jaz side browser, then reopen Voice settings.'
        : 'Claude returned HTTP ' + response.status + '.')
    }
    return response
  }

  async function account(signal?: AbortSignal): Promise<{ organization: string; model?: string; status: ClaudeVoiceStatus }> {
    const organizations = await (await request('organizations', undefined, signal)).json() as { uuid: string }[]
    const active = document.cookie.split('; ').find((entry) => entry.startsWith('lastActiveOrg='))?.slice('lastActiveOrg='.length)
    const organization = organizations.find((entry) => entry.uuid === active)?.uuid ?? (organizations.length === 1 ? organizations[0].uuid : '')
    if (!organization) {
      throw new Error('Choose your Claude account in the Jaz side browser, then reopen Voice settings.')
    }
    const catalog = await (await request('organizations/' + organization + '/model_selector/voice', undefined, signal)).json() as ModelCatalog
    const model = catalog.model_selector_config.find((entry) => entry.id === 'voice')?.models
      .find((model) => model.name === 'Opus' && !model.disabled)?.id
    return { organization, model, status: { available: Boolean(model), ...(model ? {} : { reason: 'Opus voice is unavailable for this Claude account.' }) } }
  }

  function close(id: string): void {
    starting.get(id)?.abort()
    starting.delete(id)
    const call = calls.get(id)
    calls.delete(id)
    if (!call) {
      return
    }
    clearInterval(call.keepAlive)
    if (call.socket.readyState === WebSocket.OPEN) {
      call.socket.send(JSON.stringify({ type: 'close_stream' }))
    }
    call.socket.close()
  }

  async function execute(command: ClaudeVoiceCommand): Promise<ClaudeVoiceStatus | string | undefined> {
    switch (command.type) {
      case 'status':
        try {
          return (await account()).status
        } catch (error) {
          return { available: false, reason: (error as Error).message }
        }
      case 'start': {
        close(command.id)
        const abort = new AbortController()
        starting.set(command.id, abort)
        const { organization, model, status } = await account(abort.signal)
        if (!model) {
          throw new Error(status.reason)
        }
        const conversation = crypto.randomUUID()
        await request('organizations/' + organization + '/chat_conversations', {
          uuid: conversation,
          name: 'Jaz voice',
          is_temporary: true,
        }, abort.signal)
        abort.signal.throwIfAborted()
        starting.delete(command.id)
        const query = new URLSearchParams({
          input_encoding: 'opus', input_sample_rate: '16000', input_channels: '1', output_format: 'pcm_16000',
          language: 'en', timezone: Intl.DateTimeFormat().resolvedOptions().timeZone, tts_speed: '1.00',
          server_interrupt_enabled: 'true', client_aec: 'true', client_platform: 'web_claude_ai',
          entry_source: 'chat_button', connect_reason: 'initial', capabilities: 'webrtc_media,client_tools,text_output',
          is_temporary: 'true', chat_memory_mode: 'disabled', model, voice: command.options.voice,
        })
        const socket = new WebSocket(location.origin.replace(/^http/, 'ws') + '/api/ws/voice/webrtc/organizations/' + organization + '/chat_conversations/' + conversation + '?' + query)
        const call: Call = { socket, organization, conversation, keepAlive: setInterval(() => {
          if (socket.readyState === WebSocket.OPEN) {
            socket.send(JSON.stringify({ type: 'keep_alive' }))
          }
        }, 4000) }
        calls.set(command.id, call)
        return await new Promise<string>((resolve, reject) => {
          const timer = setTimeout(() => {
            close(command.id)
            reject(new Error('Claude voice connection timed out.'))
          }, 30000)
          socket.onmessage = ({ data }) => {
            if (typeof data !== 'string') {
              return
            }
            try {
              const event = JSON.parse(data)
              if (event.type === 'session_server_initialized') {
                if (event.data?.protocol !== '2.1' || event.data?.media !== 'webrtc') {
                  throw new Error('Claude returned an unsupported voice protocol.')
                }
                socket.send(JSON.stringify({ type: 'tools_register', data: { tools: [{
                  name: 'jaz_agent',
                  description: 'Ask the selected Jaz agent to perform an action, research current facts, create an artifact, inspect files, or do substantial analysis. Also send corrections to active work and replies to its pending questions. The agent works in the background. This tool returns acceptance; later observations carry its real progress and answer. Do not claim completion before those observations. Keep ordinary conversation and speech preferences here.',
                  input_schema: { type: 'object', properties: { request: { type: 'string' } }, required: ['request'], additionalProperties: false },
                }] } }))
                socket.send(JSON.stringify({ type: 'user_message', data: { kind: 'observation', respond: false, text: command.options.context } }))
                socket.send(JSON.stringify({ type: 'media_offer', data: { sdp: command.options.sdp } }))
              }
              if (event.type === 'media_answer' && typeof event.data?.sdp === 'string') {
                clearTimeout(timer)
                resolve(event.data.sdp)
              }
              // Claude marks host observations inside its human-message envelope.
              const content = event.type === 'message_complete' && event.data?.sender === 'human' ? event.data.content : undefined
              const observation = Array.isArray(content) && content.some((block: { type: string; text?: string }) => block.type === 'text' && block.text?.includes('<untrusted_external_data source="client_observation">'))
              if (!observation) {
                ipcRenderer.send(CLAUDE_VOICE_CHANNEL + ':event', command.id, data)
              }
            } catch (error) {
              ipcRenderer.send(CLAUDE_VOICE_CHANNEL + ':event', command.id, JSON.stringify({ type: 'error', data: { display_message: (error as Error).message } }))
              clearTimeout(timer)
              close(command.id)
              reject(error)
            }
          }
          socket.onclose = () => {
            clearTimeout(timer)
            clearInterval(call.keepAlive)
            if (calls.get(command.id) === call) {
              calls.delete(command.id)
              ipcRenderer.send(CLAUDE_VOICE_CHANNEL + ':event', command.id, JSON.stringify({ type: 'jaz_disconnected' }))
            }
            reject(new Error('Claude voice disconnected. Start voice again to reconnect.'))
          }
          socket.onerror = () => {
            clearTimeout(timer)
            ipcRenderer.send(CLAUDE_VOICE_CHANNEL + ':event', command.id, JSON.stringify({ type: 'error', data: { display_message: 'Claude voice could not connect. Check your Claude sign-in.' } }))
            close(command.id)
            reject(new Error('Claude voice could not connect. Check your Claude sign-in.'))
          }
        })
      }
      case 'close':
        close(command.id)
        return
      case 'send': {
        const call = calls.get(command.id)
        if (call?.socket.readyState !== WebSocket.OPEN) {
          throw new Error('Claude voice is disconnected.')
        }
        call.socket.send(command.data)
        return
      }
      case 'result': {
        const call = calls.get(command.id)
        if (!call) {
          throw new Error('Claude voice is disconnected.')
        }
        await request('organizations/' + call.organization + '/chat_conversations/' + call.conversation + '/tool_result', {
          tool_use_id: command.toolUseId,
          content: [{ type: 'text', text: command.text }],
          is_error: command.isError,
        })
      }
    }
  }

  ipcRenderer.on(CLAUDE_VOICE_CHANNEL + ':request', (_event, requestId: number, command: ClaudeVoiceCommand) => {
    void execute(command).then((value) => {
      ipcRenderer.send(CLAUDE_VOICE_CHANNEL + ':reply', { requestId, value } satisfies ClaudeVoiceReply)
    }, (error: Error) => {
      if (command.type === 'start') {
        close(command.id)
      }
      ipcRenderer.send(CLAUDE_VOICE_CHANNEL + ':reply', { requestId, error: error.message } satisfies ClaudeVoiceReply)
    })
  })
  window.addEventListener('beforeunload', () => {
    for (const id of new Set([...calls.keys(), ...starting.keys()])) {
      close(id)
    }
  })
}
