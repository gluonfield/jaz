import type { Attachment } from './api/types'
import type { ComposerContext } from './messageContext'
export type { BrowserAnnotation, ComposerContext, MessageContextInput } from './messageContext'
export { contextAttachmentIDs, contextInputs, contextLabel, contextPreviewText } from './messageContext'

export interface SendMessageOptions {
  planRequested?: boolean
  files?: File[]
  attachments?: Attachment[]
  contexts?: ComposerContext[]
}
