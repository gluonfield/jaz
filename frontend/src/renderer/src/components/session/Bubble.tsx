import { FileText } from 'lucide-react'
import { memo } from 'react'
import type { ChatMessage, MessageBlock } from '@/lib/api/types'
import { browserAnnotationFromJSON } from '@/lib/messageContext'
import type { ComposerContext } from '@/lib/messageContext'
import { ArtifactBlock } from './ArtifactBlock'
import { AssistantMarkdown } from './AssistantMarkdown'
import { MentionText } from './mentions'
import { MessageContexts } from './MessageContexts'
import { ThinkingBlock } from './ThinkingBlock'
import { ToolCallCard } from './ToolCallCard'
import { isArtifactToolName, isHiddenToolName } from './toolVisibility'

function messageText(message: ChatMessage): string {
  // Each text block is a separate utterance; join as paragraphs so block
  // boundaries don't fuse sentences together ("…intact.Updated…").
  const text = message.blocks
    ?.filter((block) => block.type === 'text')
    .map((block) => (block.text ?? '').trim())
    .filter(Boolean)
    .join('\n\n')
  return text || message.content
}

function messageContexts(message: ChatMessage): ComposerContext[] {
  return (
    message.blocks?.flatMap<ComposerContext>((block, index) => {
      if (block.type === 'quote') {
        const text = (block.text ?? '').trim()
        return text ? [{ id: `${message.seq}-selection-${index}`, type: 'selection' as const, text }] : []
      }
      if (block.type === 'browser_annotation') {
        const annotation = browserAnnotationFromJSON(block.input_json)
        return annotation
          ? [{
              id: `${message.seq}-annotation-${index}`,
              type: 'browser_annotation' as const,
              browser_annotation: annotation,
            }]
          : []
      }
      return []
    }) ?? []
  )
}

function messageReasoning(message: ChatMessage): string {
  const text = message.blocks
    ?.filter((block) => block.type === 'reasoning')
    .map((block) => (block.text ?? '').trim())
    .filter(Boolean)
    .join('\n\n')
  return text || message.reasoning || ''
}

function isVisibleToolBlock(block: MessageBlock): block is Extract<MessageBlock, { type: 'tool' }> {
  return block.type === 'tool' && !isHiddenToolName(block.name)
}

function formatAttachmentSize(size?: number): string {
  if (!size) return ''
  if (size < 1024) return `${size} B`
  if (size < 1024 * 1024) return `${Math.round(size / 1024)} KB`
  return `${(size / (1024 * 1024)).toFixed(1)} MB`
}

function MessageAttachments({ message }: { message: ChatMessage }) {
  const attachments = message.blocks?.filter((block) => block.type === 'attachment') ?? []
  if (!attachments.length) return null
  return (
    <div className="mt-2 flex flex-wrap gap-1">
      {attachments.map((attachment) => (
        <span
          key={attachment.id}
          className="inline-flex max-w-full items-center gap-1.5 rounded-full bg-bg px-2.5 py-1 text-xs text-ink-2"
          title={attachment.server_path ?? attachment.uri}
        >
          <FileText size={13} className="shrink-0 text-primary" />
          <span className="max-w-[220px] truncate text-ink">{attachment.name}</span>
          <span className="shrink-0 text-ink-3">{formatAttachmentSize(attachment.size)}</span>
        </span>
      ))}
    </div>
  )
}

export const Bubble = memo(function Bubble({
  message,
  onArtifactPrompt,
}: {
  message: ChatMessage
  onArtifactPrompt?: (text: string) => void
}) {
  switch (message.role) {
    case 'user':
      return (
        <div className="flex justify-end">
          <div className="min-w-0 max-w-[84%] rounded-card bg-surface px-3.5 py-2.5 text-sm whitespace-pre-wrap [overflow-wrap:break-word] select-text">
            <MessageContexts contexts={messageContexts(message)} />
            <MentionText text={messageText(message)} />
            <MessageAttachments message={message} />
          </div>
        </div>
      )
    case 'assistant': {
      const text = messageText(message)
      const reasoning = messageReasoning(message)
      return (
        <div className="flex min-w-0 max-w-[var(--prose-max)] flex-col gap-2">
          <ThinkingBlock text={reasoning} />
          {text ? <AssistantMarkdown text={text} /> : null}
          {message.blocks
            ?.filter(isVisibleToolBlock)
            .map((block) =>
              isArtifactToolName(block.name) ? (
                <ArtifactBlock
                  key={block.id}
                  args={block.input_json}
                  result={block.result}
                  pending={block.result === undefined || block.result === ''}
                  onSendPrompt={onArtifactPrompt}
                />
              ) : (
                <ToolCallCard
                  key={block.id}
                  name={block.name}
                  args={block.input_json}
                  result={block.result}
                  pending={block.result === undefined || block.result === ''}
                />
              ),
            )}
        </div>
      )
    }
    // system/developer prompts are plumbing, not conversation — never shown
    default:
      return null
  }
})
