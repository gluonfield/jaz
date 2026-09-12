import type { VoiceMessage } from '@/lib/api/types'
import type { VoiceEvent } from '@/lib/voice/protocol'

export class VoiceRecorder {
  readonly callId = crypto.randomUUID()
  private active = new Map<string, VoiceMessage>()
  private lastRole = ''
  private pending: { id: string; role: 'user' | 'assistant'; text: string; at?: number }[] = []
  private delivered = new Set<string>()

  accept(event: Extract<VoiceEvent, { type: 'transcript' | 'transcript_done' }>): VoiceMessage | undefined {
    if (event.type === 'transcript' && !event.turnBased && this.lastRole !== event.role) {
      this.active.delete(event.role)
    }
    this.lastRole = event.role
    const previous = this.active.get(event.role)
    const message: VoiceMessage = {
      id: previous?.id ?? crypto.randomUUID(),
      call_id: this.callId,
      role: event.role,
      text: event.type === 'transcript_done' ? event.text : (previous?.text ?? '') + event.text,
      at: previous?.at ?? new Date().toISOString(),
    }
    if (event.type === 'transcript_done') {
      this.active.delete(event.role)
      const index = this.pending.findIndex((turn) => turn.id === message.id)
      if (!this.delivered.has(message.id)) {
        const final = { id: message.id, role: message.role, text: message.text }
        this.pending = this.pending.filter((turn) => turn.id !== message.id)
        this.pending.splice(index < 0 ? this.pending.length : index, 0, final)
      }
      this.delivered.delete(message.id)
    } else {
      this.active.set(event.role, message)
      this.pending.push({ id: message.id, role: event.role, text: event.text, at: event.at })
    }
    return message.text.trim() ? message : undefined
  }

  request(delegation: Extract<VoiceEvent, { type: 'delegation' }>): { text: string; context: string } | undefined {
    const included = this.pending.filter((turn) => delegation.at === undefined || turn.at === undefined || turn.at <= delegation.at)
    if (!delegation.text && !included.some((turn) => turn.role === 'user' && turn.text.trim())) {
      return
    }
    const lines: { role: 'user' | 'assistant'; text: string }[] = []
    for (const turn of included) {
      const previous = lines.at(-1)
      if (previous?.role === turn.role) {
        previous.text += turn.text
      }
      else {
        lines.push({ role: turn.role, text: turn.text })
      }
    }
    for (const turn of included) {
      this.delivered.add(turn.id)
    }
    const history = lines.map((turn) => `${turn.role === 'user' ? 'User' : 'Voice'}: ${turn.text}`).join('\n')
    this.pending = this.pending.filter((turn) => !included.includes(turn))
    return {
      text: delegation.text || lines.findLast((turn) => turn.role === 'user')?.text.trim() || 'Please handle the latest spoken request.',
      context: history,
    }
  }
}
