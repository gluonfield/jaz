import type { ChatMessage } from '@/lib/api/types'

// Each text block is a separate utterance; join as paragraphs so block
// boundaries don't fuse sentences together ("…intact.Updated…").
export function messageText(message: ChatMessage): string {
  const text = message.blocks
    ?.filter((block) => block.type === 'text')
    .map((block) => (block.text ?? '').trim())
    .filter(Boolean)
    .join('\n\n')
  return text || message.content
}
