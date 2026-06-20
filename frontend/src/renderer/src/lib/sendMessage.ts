import type { Attachment } from './api/types'

export interface ComposerQuote {
  id: string
  text: string
}

export interface SendMessageOptions {
  planRequested?: boolean
  files?: File[]
  attachments?: Attachment[]
  quotes?: ComposerQuote[]
}
