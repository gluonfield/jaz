import type { Attachment, MessageBlock } from './api/types'
import type { MessageContextInput } from './messageContext'

type MessageAttachmentInput = Pick<Attachment, 'name'> & Partial<Attachment>

export function userInputMessageBlocks(
  content: string,
  contexts: MessageContextInput[] = [],
  attachments: MessageAttachmentInput[] = [],
): MessageBlock[] {
  return [
    ...contexts.flatMap<MessageBlock>((context) =>
      context.type === 'selection'
        ? context.text ? [{ type: 'quote', text: context.text, comment: context.comment }] : []
        : [{ type: 'browser_annotation', input_json: JSON.stringify(context.browser_annotation ?? {}) }],
    ),
    { type: 'text', text: content },
    ...attachments.flatMap<MessageBlock>((attachment) =>
      attachment.id
        ? [{
            type: 'attachment',
            id: attachment.id,
            name: attachment.name,
            uri: attachment.uri,
            mime_type: attachment.mime_type,
            size: attachment.size,
          }]
        : [],
    ),
  ]
}
