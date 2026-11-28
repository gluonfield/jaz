import type { SessionMessages } from '@/lib/api/types'

export interface LiveExchange {
  user: string
  at: string
  baselineMessageSeq: number
  baselineEventSeq: number
  planRequested: boolean
  goalRequested: boolean
  reasoning: string
  assistant: string
  tools: LiveTool[]
  error?: string
}

export function hasCompletedLiveReply(live: LiveExchange, history: SessionMessages): boolean {
  if (history.session.status !== 'idle' || history.acp_state !== 'idle') return false
  return history.messages.some((message) => message.role === 'assistant' && message.seq > live.baselineMessageSeq) ||
    history.events.some((event) =>
      event.session_id === history.session.id &&
      event.seq != null &&
      event.seq > live.baselineEventSeq &&
      event.type === 'acp_message' &&
      Boolean(event.content?.trim()) &&
      event.acp?.id === history.session.id,
    )
}

export interface LiveTool {
  key: string
  name: string
  args?: string
  result?: string
}
