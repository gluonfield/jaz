import { useCallback, useLayoutEffect, useRef, useState } from 'react'
import type { Attachment } from '@/lib/api/types'
import type { ComposerDraftStorage } from './useComposerDraft'

export type ComposerAttachment = Partial<Attachment> & Pick<Attachment, 'name'> & {
  localId: string
  file?: File
  uploading?: boolean
  error?: string
}

function attachmentStore(kind: ComposerDraftStorage): Storage {
  return kind === 'local' ? localStorage : sessionStorage
}

function attachmentKey(key: string | undefined): string {
  return key ? `${key}.attachments` : ''
}

function storedAttachment(value: unknown): Attachment | null {
  if (!value || typeof value !== 'object') return null
  const raw = value as Record<string, unknown>
  const id = typeof raw.id === 'string' ? raw.id : ''
  const name = typeof raw.name === 'string' ? raw.name : ''
  const uri = typeof raw.uri === 'string' ? raw.uri : ''
  if (!id || !name || !uri) return null
  return {
    id,
    name,
    uri,
    ...(typeof raw.mime_type === 'string' ? { mime_type: raw.mime_type } : {}),
    ...(typeof raw.size === 'number' && Number.isFinite(raw.size) ? { size: raw.size } : {}),
    ...(typeof raw.server_path === 'string' ? { server_path: raw.server_path } : {}),
  }
}

function uploadedAttachment({ localId: _localId, file: _file, uploading, error, ...attachment }: ComposerAttachment): Attachment | null {
  if (uploading || error || !attachment.id || !attachment.uri) return null
  return attachment as Attachment
}

function readAttachments(key: string | undefined, storage: ComposerDraftStorage): ComposerAttachment[] {
  const storedKey = attachmentKey(key)
  if (!storedKey) return []
  try {
    const parsed = JSON.parse(attachmentStore(storage).getItem(storedKey) ?? '[]') as unknown
    if (!Array.isArray(parsed)) return []
    return parsed.flatMap((value) => {
      const attachment = storedAttachment(value)
      return attachment ? [{ ...attachment, localId: attachment.id }] : []
    })
  } catch {
    return []
  }
}

function writeAttachments(key: string | undefined, storage: ComposerDraftStorage, items: ComposerAttachment[]): void {
  const storedKey = attachmentKey(key)
  if (!storedKey) return
  try {
    const attachments = items.flatMap((item) => {
      const attachment = uploadedAttachment(item)
      return attachment ? [attachment] : []
    })
    if (attachments.length === 0) {
      attachmentStore(storage).removeItem(storedKey)
      return
    }
    attachmentStore(storage).setItem(storedKey, JSON.stringify(attachments))
  } catch {
    // Draft persistence must never block composing.
  }
}

export function useComposerAttachments({
  storageKey,
  storage,
  disabled,
  onUploadAttachment,
}: {
  storageKey?: string
  storage: ComposerDraftStorage
  disabled?: boolean
  onUploadAttachment?: (file: File) => Promise<Attachment>
}) {
  const [attachments, setAttachments] = useState<ComposerAttachment[]>(() =>
    readAttachments(storageKey, storage),
  )
  const attachmentsRef = useRef(attachments)

  useLayoutEffect(() => {
    const next = readAttachments(storageKey, storage)
    attachmentsRef.current = next
    setAttachments(next)
  }, [storage, storageKey])

  const commitAttachments = useCallback((next: ComposerAttachment[]) => {
    attachmentsRef.current = next
    setAttachments(next)
    writeAttachments(storageKey, storage, next)
  }, [storage, storageKey])

  const addFiles = useCallback((files: File[]) => {
    if (disabled || files.length === 0) return
    const items: ComposerAttachment[] = files.map((file) => ({
      localId: crypto.randomUUID(),
      name: file.name,
      size: file.size,
      ...(file.type ? { mime_type: file.type } : {}),
      file,
      uploading: Boolean(onUploadAttachment),
    }))
    commitAttachments([...attachmentsRef.current, ...items])
    if (!onUploadAttachment) return
    for (const item of items) {
      void onUploadAttachment(item.file!).then(
        (attachment) => {
          commitAttachments(attachmentsRef.current.map((current) =>
            current.localId === item.localId ? { ...attachment, localId: item.localId } : current,
          ))
        },
        (error) => {
          commitAttachments(attachmentsRef.current.map((current) =>
            current.localId === item.localId
              ? { ...current, uploading: false, error: (error as Error).message || 'Upload failed' }
              : current,
          ))
        },
      )
    }
  }, [commitAttachments, disabled, onUploadAttachment])

  const removeAttachment = useCallback((localId: string) => {
    commitAttachments(attachmentsRef.current.filter((item) => item.localId !== localId))
  }, [commitAttachments])

  const clearAttachments = useCallback(() => {
    commitAttachments([])
  }, [commitAttachments])

  return {
    attachments,
    addFiles,
    removeAttachment,
    clearAttachments,
    busy: attachments.some((attachment) => attachment.uploading || attachment.error),
    files: attachments.flatMap((attachment) => attachment.file ? [attachment.file] : []),
    uploaded: attachments.flatMap((attachment) => {
      const uploaded = uploadedAttachment(attachment)
      return uploaded ? [uploaded] : []
    }),
  }
}
