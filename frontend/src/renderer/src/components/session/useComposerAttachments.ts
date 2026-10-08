import { useCallback, useLayoutEffect, useRef, useState } from 'react'
import { subscribeComposerDraft } from '@/components/session/composerDraftChanges'
import type { Attachment } from '@/lib/api/types'
import type { ComposerDraftStorage } from './useComposerDraft'
import {
  loadAttachmentDraft,
  saveAttachmentDraft,
} from '@/components/session/composerAttachmentDraftStore'
import {
  pendingUploadFile,
  uploadedAttachment,
  withUploadedAttachment,
  type ComposerAttachment,
} from '@/components/session/composerAttachmentTypes'

export type { ComposerAttachment } from '@/components/session/composerAttachmentTypes'

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
  const [attachments, setAttachments] = useState<ComposerAttachment[]>([])
  const attachmentsRef = useRef(attachments)
  const revisionRef = useRef(0)
  const mountedRef = useRef(true)

  useLayoutEffect(() => {
    mountedRef.current = true
    return () => {
      mountedRef.current = false
    }
  }, [])

  useLayoutEffect(() => {
    let cancelled = false
    const revision = revisionRef.current
    attachmentsRef.current = []
    setAttachments([])
    void loadAttachmentDraft(storageKey, storage).then((restored) => {
      if (cancelled || revisionRef.current !== revision) return
      attachmentsRef.current = restored
      setAttachments(restored)
    }, () => {})
    const unsubscribe = subscribeComposerDraft(storage, storageKey, (change) => {
      if (change.field === 'attachments' && attachmentsRef.current !== change.value) {
        revisionRef.current += 1
        attachmentsRef.current = change.value
        setAttachments(change.value)
      }
    })
    return () => {
      cancelled = true
      unsubscribe()
    }
  }, [storage, storageKey])

  const commitAttachments = useCallback((next: ComposerAttachment[]) => {
    revisionRef.current += 1
    attachmentsRef.current = next
    setAttachments(next)
    void saveAttachmentDraft(storageKey, storage, next).catch(() => {})
  }, [storage, storageKey])

  const addFiles = useCallback((files: File[], error?: string) => {
    if (disabled || files.length === 0) return
    const items: ComposerAttachment[] = files.map((file) => ({
      localId: crypto.randomUUID(),
      name: file.name,
      ...(error ? { error } : {
        size: file.size,
        ...(file.type ? { mime_type: file.type } : {}),
        file,
        uploading: Boolean(onUploadAttachment),
      }),
    }))
    commitAttachments([...attachmentsRef.current, ...items])
    if (!onUploadAttachment || error) return
    for (const item of items) {
      const file = pendingUploadFile(item)
      if (!file) continue
      void onUploadAttachment(file).then(
        (attachment) => {
          if (!mountedRef.current) return
          commitAttachments(attachmentsRef.current.map((current) =>
            current.localId === item.localId
              ? withUploadedAttachment(current, attachment)
              : current,
          ))
        },
        (error) => {
          if (!mountedRef.current) return
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
    currentAttachments: () => attachmentsRef.current,
    replaceAttachments: commitAttachments,
    busy: attachments.some((attachment) => attachment.uploading || attachment.error),
    files: attachments.flatMap((attachment) => {
      const file = pendingUploadFile(attachment)
      return file ? [file] : []
    }),
    uploaded: attachments.flatMap((attachment) => {
      const uploaded = uploadedAttachment(attachment)
      return uploaded ? [uploaded] : []
    }),
  }
}
