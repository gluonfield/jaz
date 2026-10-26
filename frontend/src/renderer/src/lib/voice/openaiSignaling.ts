import { connectVoice, type VoiceProvider } from '@/lib/api/liveVoice'
import { decodeVoiceEvent, voiceContextEvents, type VoiceEvent } from '@/lib/voice/protocol'
import type { VoiceTrace } from '@/lib/voice/diagnostics'
import type { VoiceSignaling } from '@/lib/voice/signaling'

type ContextAppend = {
  event: ReturnType<typeof voiceContextEvents>[number]
  sourceSeq?: number
  timer?: ReturnType<typeof setTimeout>
}

export class OpenAIVoiceSignaling implements VoiceSignaling {
  private channel: RTCDataChannel
  private abort = new AbortController()
  private provider?: Exclude<VoiceProvider, 'claude'>
  private appends = new Map<string, ContextAppend[]>()
  private ready = false
  private ending = false
  private closed = false
  private timer?: ReturnType<typeof setTimeout>

  constructor(private peer: RTCPeerConnection, private onEvent: (event: VoiceEvent) => void, private trace: VoiceTrace) {
    this.channel = peer.createDataChannel('oai-events')
    this.channel.onmessage = ({ data }: MessageEvent<string>) => {
      if (this.closed) return
      try {
        const raw = JSON.parse(data)
        if (['delegation.context.appended', 'session.context.appended', 'session.thinking.appended', 'session.commentary.appended'].includes(raw.type)) {
          const key = this.provider === 'openai' ? raw.delegation_item_id ?? '' : raw.client_event_id
          const queue = this.appends.get(key)
          const sent = queue?.shift()
          clearTimeout(sent?.timer)
          this.trace('append_ack', { event_id: sent?.event.event_id, source_seq: sent?.sourceSeq, request_id: raw.delegation_item_id ?? raw.delegation_id, provider_start_ms: raw.start_ms, provider_end_ms: raw.end_ms, detail: sent ? raw.type : `Unmatched ${raw.type}` })
          if (queue?.length) {
            this.sendContext(key)
          } else {
            this.appends.delete(key)
          }
        }
        if (raw.type === 'turn.created' || raw.type === 'turn.done') {
          this.trace(raw.type, { user_turn_id: raw.turn?.id, provider_start_ms: raw.turn?.start_ms, provider_end_ms: raw.turn?.end_ms, detail: raw.turn?.role })
        }
        const event = decodeVoiceEvent(data)
        if (!event || this.closed) return
        if (event.type === 'error') {
          this.trace('provider_error', { detail: event.message.slice(0, 512) })
        }
        if (event.type === 'ready') {
          this.ready = true
          clearTimeout(this.timer)
        }
        if (event.type === 'closed') this.dispose()
        this.onEvent(event)
      } catch {
        this.fail('The voice provider sent an invalid event.')
      }
    }
    this.channel.onclose = () => {
      if (this.closed) return
      if (this.ending) {
        this.dispose()
        this.onEvent({ type: 'closed' })
      } else this.fail('Voice disconnected. Start voice again to reconnect.')
    }
  }

  async start(sessionId: string, sdp: string, context: string): Promise<void> {
    const result = await connectVoice(sessionId, sdp, context, this.abort.signal)
    if (result.provider === 'claude') {
      throw new Error('Voice provider changed. Start voice again.')
    }
    this.provider = result.provider
    if (!this.closed) {
      await this.peer.setRemoteDescription({ type: 'answer', sdp: result.sdp })
    }
  }

  append(text: string, speak = false, delegationId?: string, sourceSeq?: number) {
    if (this.ending || this.closed) return
    if (!this.ready || !this.provider || this.channel.readyState !== 'open') {
      this.trace('context_failed', { request_id: delegationId, source_seq: sourceSeq })
      this.fail('Voice could not receive the agent result. It remains available in the chat.')
      return
    }
    if (sourceSeq !== undefined) {
      this.trace('result_available', { request_id: delegationId, source_seq: sourceSeq })
    }
    for (const event of voiceContextEvents(this.provider, text, speak, delegationId)) {
      if (this.closed) return
      const key = this.provider === 'openai' ? delegationId ?? '' : event.event_id
      const queue = this.appends.get(key) ?? []
      queue.push({ event, sourceSeq })
      this.appends.set(key, queue)
      if (queue.length === 1) {
        this.sendContext(key)
      }
    }
  }

  private sendContext(key: string) {
    const next = this.appends.get(key)?.[0]
    if (!next) return
    const { event, sourceSeq } = next
    try {
      this.channel.send(JSON.stringify(event))
      this.trace('context_sent', { event_id: event.event_id, request_id: 'delegation_item_id' in event ? event.delegation_item_id : event.delegation_id ?? undefined, source_seq: sourceSeq, detail: event.type })
      next.timer = setTimeout(() => this.fail('Voice did not acknowledge the agent update. It remains available in the chat.'), 15000)
    } catch {
      this.fail('Voice could not receive the agent result. It remains available in the chat.')
    }
  }

  end() {
    if (this.closed || this.ending) return
    this.ending = true
    clearTimeout(this.timer)
    if (this.ready && this.channel.readyState === 'open') {
      this.channel.send(JSON.stringify({ type: 'session.close' }))
      this.timer = setTimeout(() => {
        this.dispose()
        this.onEvent({ type: 'closed' })
      }, 3000)
    } else {
      this.dispose()
      this.onEvent({ type: 'closed' })
    }
  }

  dispose(): void {
    if (this.closed) {
      return
    }
    this.closed = true
    clearTimeout(this.timer)
    for (const queue of this.appends.values()) {
      clearTimeout(queue[0]?.timer)
    }
    this.appends.clear()
    this.abort.abort()
    this.channel.close()
  }

  private fail(message: string): void {
    if (this.closed) {
      return
    }
    this.trace('connection_failed', { detail: message })
    this.dispose()
    this.onEvent({ type: 'error', message })
  }
}
