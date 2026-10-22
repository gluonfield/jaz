import type { VoiceProvider } from '@/lib/api/liveVoice'

export type VoiceEvent =
  | { type: 'ready' | 'closed' }
  | { type: 'turn_started'; role: 'user' | 'assistant'; userTurnId: string }
  | { type: 'transcript'; role: 'user' | 'assistant'; text: string; userTurnId?: string; at?: number; turnBased?: boolean }
  | { type: 'transcript_done'; role: 'user' | 'assistant'; text: string; userTurnId?: string }
  | { type: 'delegation'; id: string; userTurnId?: string; text?: string; at?: number; turnBased?: boolean }
  | { type: 'error'; message: string }

type WireEvent = {
  type: string
  delta?: string
  start_ms?: number
  offset_ms?: number
  item?: { user_bidi_turn_id?: string; id?: string; target?: string; text?: string; content?: { type: string; text?: string }[] }
  turn?: { id?: string; role?: string; transcript?: string }
  delegation?: { id: string; target: string }
  error?: { message?: string }
  message?: string
}

export function decodeVoiceEvent(data: string): VoiceEvent | undefined {
  const event = JSON.parse(data) as WireEvent
  switch (event.type) {
    case 'session.started':
      return { type: 'ready' }
    case 'session.closed':
      return { type: 'closed' }
    case 'input_transcript.added':
    case 'output_transcript.added':
      return { type: 'transcript', role: event.type === 'input_transcript.added' ? 'user' : 'assistant', text: event.item?.text ?? '', turnBased: true, ...(event.item?.user_bidi_turn_id ? { userTurnId: event.item.user_bidi_turn_id } : {}) }
    case 'turn.created':
      if (event.turn?.id && (event.turn.role === 'user' || event.turn.role === 'assistant')) return { type: 'turn_started', role: event.turn.role, userTurnId: event.turn.id }
      return
    case 'turn.done':
      if (event.turn?.role === 'user' || event.turn?.role === 'assistant') return { type: 'transcript_done', role: event.turn.role, text: event.turn.transcript ?? '', ...(event.turn.id ? { userTurnId: event.turn.id } : {}) }
      return
    case 'session.input_transcript.delta':
    case 'session.output_transcript.delta':
      return { type: 'transcript', role: event.type === 'session.input_transcript.delta' ? 'user' : 'assistant', text: event.delta ?? '', at: event.start_ms }
    case 'delegation.created':
      if (event.item?.target !== 'client' || !event.item.id) return
      return {
        type: 'delegation',
        id: event.item.id,
        turnBased: true,
        ...(event.item.user_bidi_turn_id ? { userTurnId: event.item.user_bidi_turn_id } : {}),
        text: event.item.content?.filter((part) => part.type === 'input_text').map((part) => part.text ?? '').join('\n'),
      }
    case 'session.delegation.created':
      if (event.delegation?.target !== 'client' || !event.delegation.id) return
      return { type: 'delegation', id: event.delegation.id, at: event.offset_ms }
    case 'error':
    case 'session.error':
      return { type: 'error', message: event.error?.message ?? event.message ?? 'The voice connection reported an error.' }
  }
}

export function voiceContextEvents(provider: VoiceProvider, text: string, speak: boolean, delegationId?: string) {
  return utf8Chunks(text, 480).map((chunk) => provider === 'openai' ? {
    type: delegationId ? 'delegation.context.append' : 'session.context.append',
    ...(delegationId ? { delegation_item_id: delegationId } : {}),
    event_id: crypto.randomUUID(),
    channel: speak ? 'speakable' : 'commentary',
    content: [{ type: 'input_text', text: chunk }],
  } : {
    type: speak ? 'session.commentary.append' : 'session.thinking.append',
    event_id: crypto.randomUUID(),
    delegation_id: delegationId ?? null,
    content: chunk,
  })
}

export function utf8Chunks(text: string, limit: number): string[] {
  const encoder = new TextEncoder()
  const chunks: string[] = []
  let chunk = ''
  let size = 0
  for (const character of text) {
    const bytes = encoder.encode(character).length
    if (size + bytes > limit) {
      chunks.push(chunk)
      chunk = ''
      size = 0
    }
    chunk += character
    size += bytes
  }
  if (chunk) chunks.push(chunk)
  return chunks
}
