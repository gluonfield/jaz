import type { Attachment } from './api/types'

export interface SendMessageOptions {
  planRequested?: boolean
  files?: File[]
  attachments?: Attachment[]
}
