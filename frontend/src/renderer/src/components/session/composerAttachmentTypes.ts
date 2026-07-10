import type { Attachment } from '@/lib/api/types'

export type ComposerAttachment = {
  localId: string
  name: string
  id?: string
  uri?: string
  mime_type?: string
  size?: number
  file?: File
  previewFile?: File
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

export function composerAttachmentPreviewFile(item: ComposerAttachment): File | undefined {
  return item.previewFile ?? item.file
}

export function pendingUploadFile(item: ComposerAttachment): File | null {
  return item.file ?? null
}

export function withUploadedAttachment(item: ComposerAttachment, attachment: Attachment): ComposerAttachment {
  const previewFile = item.previewFile ?? item.file
  return {
    localId: item.localId,
    id: attachment.id,
    name: attachment.name,
    ...(attachment.uri ? { uri: attachment.uri } : {}),
    ...(attachment.mime_type ? { mime_type: attachment.mime_type } : {}),
    ...(typeof attachment.size === 'number' && Number.isFinite(attachment.size) ? { size: attachment.size } : {}),
    ...(previewFile ? { previewFile } : {}),
  }
}
