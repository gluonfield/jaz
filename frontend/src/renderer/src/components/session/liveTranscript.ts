import type { ChatMessage } from '@/lib/api/types'
import { contextInputs } from '@/lib/messageContext'
import { userInputMessageBlocks } from '@/lib/messageBlocks'
import type { ComposerContext } from '@/lib/sendMessage'

export interface LiveExchange {
  user: string
  at: string
  baselineMessageSeq: number
  planRequested: boolean
  goalRequested: boolean
  contexts: ComposerContext[]
  attachments: LiveAttachment[]
  reasoning: string
  assistant: string
  tools: LiveTool[]
  error?: string
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

export function liveTranscriptMessages(
  messages: ChatMessage[],
  live: LiveExchange | null,
  active: boolean,
): ChatMessage[] {
  if (!active || !live) return messages

  const currentIndex = messages.findIndex(
    (message) => message.seq > live.baselineMessageSeq && message.role === 'user',
  )
  if (currentIndex === -1) {
    return [...messages, liveUserMessage(live, (messages.at(-1)?.seq ?? 0) + 1_000_000)]
  }
  if (messages.slice(currentIndex + 1).some((message) => message.role === 'user')) return messages

  const out = messages.slice()
  out[currentIndex] = liveUserMessage(live, messages[currentIndex].seq)
  return out
}

function liveUserMessage(live: LiveExchange, seq: number): ChatMessage {
  return {
    seq,
    role: 'user',
    content: live.user,
    blocks: userInputMessageBlocks(live.user, contextInputs(live.contexts), live.attachments),
    created_at: live.at,
  }
}
