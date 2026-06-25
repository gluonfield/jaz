import type { Attachment } from './api/types'
import { contextAttachmentIDs, contextInputs } from './messageContext'
import type { ComposerContext, MessageContextInput } from './messageContext'
export type { BrowserAnnotation, ComposerContext, MessageContextInput } from './messageContext'
export { contextAttachmentIDs, contextInputs, contextLabel, contextPreviewText } from './messageContext'

export interface SendMessageOptions {
  planRequested?: boolean
  goalRequested?: boolean
  files?: File[]
  attachments?: Attachment[]
  contexts?: ComposerContext[]
}

export type SendMessageHandler = (text: string, options?: SendMessageOptions) => void | Promise<void>

export interface PreparedSendMessage {
  contexts: MessageContextInput[]
  attachmentIds: string[]
}

export function preparedSendMessage(options: SendMessageOptions = {}, uploaded: Attachment[] = []): PreparedSendMessage {
  const contexts = options.contexts ?? []
  return {
    contexts: contextInputs(contexts),
    attachmentIds: [
      ...(options.attachments ?? []).map((attachment) => attachment.id),
      ...contextAttachmentIDs(contexts),
      ...uploaded.map((attachment) => attachment.id),
    ],
  }
}
