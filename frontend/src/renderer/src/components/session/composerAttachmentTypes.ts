import type { Attachment } from '@/lib/api/types'

export type ComposerAttachment = {
  localId: string
  name: string
  id?: string
  uri?: string
  mime_type?: string
  size?: number
  file?: File
  uploading?: boolean
  error?: string
}

export function uploadedAttachment(item: ComposerAttachment): Attachment | null {
  if (item.uploading || item.error || !item.id) return null
  return {
    id: item.id,
    name: item.name,
    ...(item.uri ? { uri: item.uri } : {}),
    ...(item.mime_type ? { mime_type: item.mime_type } : {}),
    ...(typeof item.size === 'number' && Number.isFinite(item.size) ? { size: item.size } : {}),
  }
}
