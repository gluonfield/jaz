import type { ChatMessage } from '@/lib/api/types'

export type OptimisticUserChatMessage = Omit<ChatMessage, 'seq' | 'role'> & { role: 'user' }

export interface OptimisticUserMessage {
  baselineMessageSeq: number
  message: OptimisticUserChatMessage
}

export function materializeOptimisticUserMessage(
  optimistic: OptimisticUserMessage,
  seq = 0,
): ChatMessage {
  return { ...optimistic.message, seq }
}

export function pendingOptimisticUserMessage(
  messages: ChatMessage[],
  optimistic: OptimisticUserMessage | null,
): OptimisticUserMessage | null {
  if (!optimistic) return null
  const persisted = messages.some(
    (message) => message.seq > optimistic.baselineMessageSeq && message.role === 'user',
  )
  return persisted ? null : optimistic
}

export function optimisticTranscriptMessages(
  messages: ChatMessage[],
  optimistic: OptimisticUserMessage | null,
): ChatMessage[] {
  if (!optimistic) return messages

  const currentIndex = messages.findIndex(
    (message) => message.seq > optimistic.baselineMessageSeq && message.role === 'user',
  )
  if (currentIndex === -1) {
    return [
      ...messages,
      materializeOptimisticUserMessage(optimistic, (messages.at(-1)?.seq ?? 0) + 1_000_000),
    ]
  }
  if (messages.slice(currentIndex + 1).some((message) => message.role === 'user')) return messages

  const out = messages.slice()
  out[currentIndex] = materializeOptimisticUserMessage(optimistic, messages[currentIndex].seq)
  return out
}
