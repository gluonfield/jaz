import { contextInputs } from '@/lib/messageContext'
import { userInputMessageBlocks } from '@/lib/messageBlocks'
import type { SessionMessages } from '@/lib/api/types'
import type { OptimisticUserMessage } from '@/lib/optimisticUserMessage'
import type { ComposerContext } from '@/lib/sendMessage'

export interface LiveExchange {
  user: string
  at: string
  baselineMessageSeq: number
  baselineEventSeq: number
  planRequested: boolean
  goalRequested: boolean
  contexts: ComposerContext[]
  attachments: LiveAttachment[]
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

export interface LiveAttachment {
  id?: string
  name: string
  uri?: string
  mime_type?: string
  size?: number
  uploading?: boolean
}

export interface LiveTool {
  key: string
  name: string
  args?: string
  result?: string
}

export function liveOptimisticUserMessage(live: LiveExchange): OptimisticUserMessage {
  return {
    baselineMessageSeq: live.baselineMessageSeq,
    message: {
      role: 'user',
      content: live.user,
      blocks: userInputMessageBlocks(live.user, contextInputs(live.contexts), live.attachments),
      created_at: live.at,
    },
  }
}
