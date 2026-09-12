import type { SessionMessages } from '@/lib/api/types'
import { coalesceSessionEvents } from '@/lib/sessionEvents'

export function voiceThreadEvents(snapshot: SessionMessages) {
  return coalesceSessionEvents(snapshot.events.filter((event) => event.session_id === snapshot.session.id && (!event.acp?.id || event.acp.id === snapshot.session.id)))
}

export function voiceChatContext(snapshot: SessionMessages, activeCallId?: string): string {
  const session = snapshot.session
  const events = voiceThreadEvents(snapshot)
  const entries = [
    ...snapshot.messages.filter((message) => message.role === 'user' || message.role === 'assistant')
      .map((message) => ({ role: message.role, text: message.content, at: message.created_at })),
    ...events.filter((event) => event.type === 'acp_message' && event.content)
      .map((event) => ({ role: 'assistant', text: event.content!, at: event.at })),
    ...events.filter((event) => event.voice && event.voice.call_id !== activeCallId)
      .map((event) => ({ role: event.voice!.role, text: event.voice!.text, at: event.voice!.at })),
  ].sort((a, b) => Date.parse(a.at) - Date.parse(b.at)).slice(-12)
  const recent: string[] = []
  let remaining = 12000
  for (const entry of entries.reverse()) {
    if (remaining <= 0) break
    const text = `${entry.role}: ${entry.text.slice(0, 3000)}`.slice(0, remaining)
    recent.unshift(text)
    remaining -= text.length + 1
  }
  return [
    'Current chat context (background only; earlier actions may already be complete):',
    `Thread: ${session.title || session.id}`,
    `Selected agent: ${session.runtime_ref?.agent || 'unknown'}`,
    `Selected agent model: ${session.model || 'unknown'}`,
    `Workspace: ${session.runtime_ref?.cwd || 'unknown'}`,
    `Agent status: ${session.status}`,
    'Recent conversation:',
    ...recent,
  ].join('\n').slice(0, 16000)
}
