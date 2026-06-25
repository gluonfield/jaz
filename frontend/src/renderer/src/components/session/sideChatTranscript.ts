import type { Attachment, ChatMessage, SessionEvent } from '@/lib/api/types'
import type { MessageContextInput } from '@/lib/messageContext'
import { userInputMessageBlocks } from '@/lib/messageBlocks'

export type SideChatTranscript = {
  messages: ChatMessage[]
  events: SessionEvent[]
}

export function latestSideChatID(events: SessionEvent[]): string {
  const latest = events
    .filter((event) => event.type === 'side_chat_message' && event.side_chat?.id)
    .sort(compareEvents)
    .at(-1)
  return latest?.side_chat?.id ?? ''
}

export function sideChatHasEvents(events: SessionEvent[], sideChatID: string): boolean {
  return events.some((event) => event.type === 'side_chat_message' && event.side_chat?.id === sideChatID)
}

export function sideChatEvents(events: SessionEvent[], sideChatID: string): SessionEvent[] {
  return [...events]
    .filter((event) => event.type === 'side_chat_message' && event.side_chat?.id === sideChatID)
    .sort(compareEvents)
}

export function sideChatTranscript(events: SessionEvent[]): SideChatTranscript {
  const out: SideChatTranscript = { messages: [], events: [] }
  let openAssistant: ChatMessage | null = null
  events.forEach((event, index) => {
    const side = event.side_chat
    if (!side) return
    const content = side.content ?? event.content ?? ''
    const role = side.role || 'assistant'
    if (role === 'thought') {
      openAssistant = null
      out.events.push(sideChatACPEvent(event, 'acp_thought', { thought: content }))
      return
    }
    if (role === 'tool') {
      openAssistant = null
      out.events.push(sideChatACPEvent(event, 'acp_tool', {
        tool_calls: [{
          id: `side-tool-${event.seq ?? index}`,
          title: content,
          status: side.status,
          updated_at: event.at,
        }],
      }))
      return
    }
    if (role === 'error') {
      openAssistant = null
      out.events.push(sideChatACPEvent(event, 'acp', { error: content, state: 'failed' }))
      return
    }
    const messageRole = role === 'user' ? 'user' : 'assistant'
    if (messageRole === 'assistant' && openAssistant) {
      openAssistant.content += content
      openAssistant.blocks = [{ type: 'text', text: openAssistant.content }]
      openAssistant.created_at = event.at || openAssistant.created_at
      return
    }
    if (messageRole === 'user') {
      openAssistant = null
      out.messages.push(sideChatUserMessage(content, side.contexts ?? [], side.attachments ?? [], event.at, event.seq ?? index))
      return
    }
    openAssistant = sideChatAssistantMessage(content, event.at, event.seq ?? index)
    out.messages.push(openAssistant)
  })
  return out
}

export function sideChatUserMessage(
  content: string,
  contexts: MessageContextInput[],
  attachments: Attachment[],
  at: string,
  seq: number,
): ChatMessage {
  return {
    seq,
    role: 'user',
    content,
    blocks: userInputMessageBlocks(content, contexts, attachments),
    created_at: at,
  }
}

export function liveSideChatSeq(events: SessionEvent[]): number {
  return (events.at(-1)?.seq ?? events.length) + 1
}

export function latestSideChatText(messages: ChatMessage[], events: SessionEvent[]): string {
  const message = messages.at(-1)
  const event = events.at(-1)
  if (!event) return message?.content ?? ''
  const eventText = event.acp?.thought ?? event.acp?.error ?? event.acp?.tool_calls?.at(-1)?.title ?? event.content ?? ''
  if (!message) return eventText
  return eventTime(event) >= messageTime(message) ? eventText || message.content : message.content
}

function sideChatACPEvent(
  event: SessionEvent,
  type: string,
  acp: Partial<NonNullable<SessionEvent['acp']>>,
): SessionEvent {
  const side = event.side_chat
  return {
    session_id: event.session_id,
    seq: event.seq,
    type,
    acp: {
      id: side?.id ?? event.session_id,
      slug: 'side-chat',
      agent: 'codex',
      session_id: side?.id ?? event.session_id,
      state: side?.status || 'running',
      ...acp,
    },
    at: event.at,
  }
}

function sideChatAssistantMessage(content: string, at: string, seq: number): ChatMessage {
  return {
    seq,
    role: 'assistant',
    content,
    blocks: [{ type: 'text', text: content }],
    created_at: at,
  }
}

function compareEvents(a: SessionEvent, b: SessionEvent): number {
  if (a.seq && b.seq && a.session_id === b.session_id) return a.seq - b.seq
  return eventTime(a) - eventTime(b) || (a.seq ?? 0) - (b.seq ?? 0)
}

function eventTime(event: SessionEvent): number {
  const at = Date.parse(event.at)
  return Number.isNaN(at) ? 0 : at
}

function messageTime(message: ChatMessage): number {
  const at = Date.parse(message.created_at)
  return Number.isNaN(at) ? 0 : at
}
