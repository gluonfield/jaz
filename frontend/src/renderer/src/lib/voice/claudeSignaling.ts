import { connectVoice } from '@/lib/api/liveVoice'
import type { ClaudeVoiceAPI } from '@shared/claudeVoice'
import type { VoiceEvent } from '@/lib/voice/protocol'
import type { VoiceTrace } from '@/lib/voice/diagnostics'
import type { VoiceSignaling } from '@/lib/voice/signaling'

type Block = { type: string; text?: string; id?: string; name?: string; input?: { request?: string } }
type WireEvent = {
  type: string
  text?: string
  data?: { sender?: string; message_uuid?: string; parent_message_uuid?: string; content?: Block[]; display_message?: string; error_code?: string }
  event?: {
    type: string
    data?: {
      message?: { id?: string; uuid?: string; parent_uuid?: string; model?: string }
      index?: number
      content_block?: Block
      delta?: { text?: string; partial_json?: string }
    }
  }
}

export class ClaudeVoiceSignaling implements VoiceSignaling {
  private id = crypto.randomUUID()
  private api: ClaudeVoiceAPI
  private abort = new AbortController()
  private unsubscribe: () => void
  private closed = false
  private assistantId?: string
  private humanId?: string
  private tool?: { id: string; name: string; index?: number; json: string }
  private tools = new Map<string, 'pending' | 'answered'>()
  private updates = Promise.resolve()

  constructor(private peer: RTCPeerConnection, private onEvent: (event: VoiceEvent) => void, private trace: VoiceTrace, private voice: string) {
    const api = window.jaz?.claudeVoice
    if (!api) {
      throw new Error('Claude voice requires Jaz Desktop.')
    }
    this.api = api
    this.unsubscribe = api.onEvent((id, data) => {
      if (id !== this.id || this.closed) {
        return
      }
      try {
        this.receive(JSON.parse(data) as WireEvent)
      } catch {
        this.fail('Claude sent an invalid voice event.')
      }
    })
  }

  async start(sessionId: string, sdp: string, context: string): Promise<void> {
    const result = await connectVoice(sessionId, sdp, context, this.abort.signal)
    if (this.closed) {
      return
    }
    if (result.provider !== 'claude' || !result.model || !result.instructions) {
      throw new Error('Voice provider changed. Start voice again.')
    }
    const answer = await this.api.start(this.id, { sdp, model: result.model, voice: this.voice, context: result.instructions })
    if (this.closed) {
      return
    }
    await this.peer.setRemoteDescription({ type: 'answer', sdp: answer })
    this.onEvent({ type: 'ready' })
  }

  private receive(event: WireEvent): void {
    if (event.type === 'jaz_disconnected') {
      this.fail('Claude voice disconnected. Start voice again to reconnect.')
      return
    }
    if (event.type === 'error') {
      this.fail(event.data?.display_message ?? 'Claude voice reported an error: ' + (event.data?.error_code ?? 'unknown'))
      return
    }
    if (event.type === 'transcript_interim' && event.text) {
      this.onEvent({ type: 'transcript', role: 'user', text: event.text, replace: true })
    }
    if (event.type === 'message_complete') {
      const message = event.data
      if (message?.sender !== 'human' && message?.sender !== 'assistant') {
        return
      }
      const text = message.content?.filter((block) => block.type === 'text').map((block) => block.text ?? '').join('') ?? ''
      const role = message.sender === 'human' ? 'user' : 'assistant'
      const userTurnId = role === 'user' ? message.message_uuid : this.assistantId
      this.onEvent({ type: 'transcript_done', role, text, userTurnId })
      return
    }
    if (event.type !== 'message_sse') {
      return
    }
    const sse = event.event
    const data = sse?.data
    switch (sse?.type) {
      case 'message_start': {
        const message = data?.message
        this.trace('voice_model', { detail: message?.model ?? 'unknown' })
        this.assistantId = message?.uuid ?? message?.id
        this.humanId = message?.parent_uuid
        this.onEvent({ type: 'turn_started', role: 'assistant', userTurnId: this.assistantId ?? crypto.randomUUID() })
        break
      }
      case 'content_block_start': {
        const block = data?.content_block
        if (block?.type === 'tool_use' && block.id && block.name) {
          this.tool = { id: block.id, name: block.name, index: data?.index, json: block.input && Object.keys(block.input).length ? JSON.stringify(block.input) : '' }
        }
        break
      }
      case 'content_block_delta':
        if (data?.delta?.text) {
          this.onEvent({ type: 'transcript', role: 'assistant', text: data.delta.text, userTurnId: this.assistantId, turnBased: true })
        }
        if (this.tool && data?.index === this.tool.index && data?.delta?.partial_json) {
          this.tool.json += data.delta.partial_json
        }
        break
      case 'content_block_stop': {
        const tool = this.tool
        if (!tool || data?.index !== tool.index) {
          return
        }
        this.tool = undefined
        const input = JSON.parse(tool.json || '{}') as { request?: string }
        if (tool.name !== 'jaz_agent' || typeof input.request !== 'string' || !input.request.trim()) {
          void this.api.result(this.id, tool.id, 'This tool requires a nonempty request for the selected Jaz agent.', true).catch((error: Error) => this.fail(error.message))
          return
        }
        if (!this.tools.has(tool.id)) {
          this.tools.set(tool.id, 'pending')
          this.onEvent({ type: 'delegation', id: tool.id, userTurnId: this.humanId, text: input.request, turnBased: true })
        }
        break
      }
    }
  }

  append(text: string, speak = false, delegationId?: string, sourceSeq?: number): void {
    if (this.closed) {
      return
    }
    this.updates = this.updates.then(async () => {
      if (this.closed) {
        return
      }
      if (delegationId && this.tools.get(delegationId) === 'pending') {
        this.tools.set(delegationId, 'answered')
        await this.api.result(this.id, delegationId, text, false)
      } else {
        await this.api.send(this.id, JSON.stringify({ type: 'user_message', data: { kind: 'observation', text, respond: speak } }))
      }
      this.trace('context_sent', { request_id: delegationId, source_seq: sourceSeq, detail: 'Claude agent update' })
    }).catch((error: Error) => this.fail(error.message))
  }

  end(): void {
    this.dispose()
    this.onEvent({ type: 'closed' })
  }

  dispose(): void {
    if (this.closed) {
      return
    }
    this.closed = true
    this.abort.abort()
    this.unsubscribe()
    void this.api.close(this.id).catch(() => {})
  }

  private fail(message: string): void {
    if (this.closed) {
      return
    }
    this.dispose()
    this.onEvent({ type: 'error', message })
  }
}
