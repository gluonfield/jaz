import { memo } from 'react'
import type { ChatMessage, MessageBlock } from '@/lib/api/types'
import { browserAnnotationFromJSON } from '@/lib/messageContext'
import type { ComposerContext } from '@/lib/messageContext'
import { ArtifactBlock } from './ArtifactBlock'
import { AssistantMarkdown } from './AssistantMarkdown'
import { MentionText } from './mentions'
import { MessageAttachments, type MessageAttachment } from './MessageAttachments'
import { MessageContexts } from './MessageContexts'
import { ThinkingBlock } from './ThinkingBlock'
import { ToolCallCard } from './ToolCallCard'
import { isArtifactToolName, isHiddenToolName } from './toolVisibility'

type ToolBlock = Extract<MessageBlock, { type: 'tool' }>

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
        if (!text) return []
        const id = `${message.seq}-selection-${index}`
        return [{ id, type: 'selection' as const, text, comment: (block.comment ?? '').trim() || undefined }]
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

export function UserBubble({
  text,
  contexts = [],
  attachments = [],
  attachmentSessionId,
}: {
  text: string
  contexts?: ComposerContext[]
  attachments?: MessageAttachment[]
  attachmentSessionId?: string
}) {
  return (
    <div className="flex justify-end">
      <div className="min-w-0 max-w-[84%] rounded-card bg-surface px-3.5 py-2.5 text-sm whitespace-pre-wrap [overflow-wrap:break-word] select-text">
        <MessageContexts contexts={contexts} />
        <MentionText text={text} />
        <MessageAttachments attachments={attachments} attachmentSessionId={attachmentSessionId} />
      </div>
    </div>
  )
}

export function AssistantBubble({
  text,
  reasoning = '',
  tools = [],
  showCopy = true,
  onArtifactPrompt,
}: {
  text: string
  reasoning?: string
  tools?: ToolBlock[]
  showCopy?: boolean
  onArtifactPrompt?: (text: string) => void
}) {
  return (
    <div className="flex min-w-0 max-w-[var(--prose-max)] flex-col gap-2">
      <ThinkingBlock text={reasoning} />
      {text ? <AssistantMarkdown text={text} showCopy={showCopy} /> : null}
      {tools.map((block) =>
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

export const Bubble = memo(function Bubble({
  message,
  showAssistantCopy = true,
  onArtifactPrompt,
  attachmentSessionId,
}: {
  message: ChatMessage
  showAssistantCopy?: boolean
  onArtifactPrompt?: (text: string) => void
  attachmentSessionId?: string
}) {
  switch (message.role) {
    case 'user':
      return (
        <UserBubble
          text={messageText(message)}
          contexts={messageContexts(message)}
          attachments={message.blocks?.filter((block) => block.type === 'attachment') ?? []}
          attachmentSessionId={attachmentSessionId}
        />
      )
    case 'assistant': {
      return (
        <AssistantBubble
          text={messageText(message)}
          reasoning={messageReasoning(message)}
          tools={message.blocks?.filter(isVisibleToolBlock) ?? []}
          showCopy={showAssistantCopy}
          onArtifactPrompt={onArtifactPrompt}
        />
      )
    }
    // system/developer prompts are plumbing, not conversation — never shown
    default:
      return null
  }
})
