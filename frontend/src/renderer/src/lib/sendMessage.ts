import type { Attachment } from './api/types'

export interface ComposerQuote {
  id: string
  text: string
  /** seq of the assistant message the selection was taken from, for context */
  sourceSeq?: number
}

export interface SendMessageOptions {
  planRequested?: boolean
  files?: File[]
  attachments?: Attachment[]
  quotes?: ComposerQuote[]
}
