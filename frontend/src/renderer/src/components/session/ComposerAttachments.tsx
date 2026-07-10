import { FileText, LoaderCircle, Paperclip, X } from 'lucide-react'
import type { RefObject } from 'react'
import { MenuRow } from '@/components/ui/Popover'
import { ImageAttachmentTile, isImageAttachment, type MessageAttachment } from './MessageAttachments'
import { composerAttachmentPreviewFile } from './composerAttachmentTypes'
import type { ComposerAttachment } from './useComposerAttachments'

export function ComposerAttachmentInput({
  disabled,
  inputRef,
  onAddFiles,
}: {
  disabled?: boolean
  inputRef: RefObject<HTMLInputElement | null>
  onAddFiles: (files: File[]) => void
}) {
  return (
    <input
      ref={inputRef}
      type="file"
      multiple
      className="hidden"
      disabled={disabled}
      onChange={(e) => {
        onAddFiles(Array.from(e.currentTarget.files ?? []))
        e.currentTarget.value = ''
      }}
    />
  )
}

export function ComposerAttachmentList({
  attachments,
  attachmentSessionId,
  onRemove,
}: {
  attachments: ComposerAttachment[]
  attachmentSessionId?: string
  onRemove: (localId: string) => void
}) {
  if (attachments.length === 0) return null
  const images = attachments.filter(isImageAttachment)
  const files = attachments.filter((attachment) => !isImageAttachment(attachment))
  return (
    <div className="flex max-w-full flex-col gap-2 px-1.5 pt-0.5">
      {images.length ? (
        <div className="flex max-w-full flex-wrap gap-2">
          {images.map((attachment) => (
            <div key={attachment.localId} className="relative max-w-full">
              <ImageAttachmentTile
                attachment={messageAttachmentFromComposer(attachment)}
                attachmentSessionId={attachmentSessionId}
              />
              <button
                type="button"
                className="absolute -top-1.5 -left-1.5 z-10 grid size-6 place-items-center rounded-full bg-bg/95 text-ink-3 shadow-sm ring-1 ring-border/70 backdrop-blur-sm transition-[background-color,color,transform] duration-150 hover:bg-surface-2 hover:text-ink active:scale-[0.96]"
                aria-label={`Remove ${attachment.name}`}
                title={`Remove ${attachment.name}`}
                onClick={() => onRemove(attachment.localId)}
              >
                <X size={12} />
              </button>
            </div>
          ))}
        </div>
      ) : null}
      {files.length ? (
        <div className="flex flex-wrap gap-1">
          {files.map((attachment) => (
            <div
              key={attachment.localId}
              title={attachment.error ?? attachment.name}
              className="flex max-w-full items-center gap-1.5 rounded-full bg-bg py-1.5 pr-1.5 pl-3 text-xs text-ink-2 transition-colors hover:bg-surface-2"
            >
              {attachment.uploading ? (
                <LoaderCircle size={13} className="shrink-0 animate-spin text-ink-3" />
              ) : (
                <FileText size={13} className={`shrink-0 ${attachment.error ? 'text-danger' : 'text-ink-3'}`} />
              )}
              <span className="max-w-[200px] truncate text-ink">{attachment.name}</span>
              {attachment.error ? <span className="shrink-0 text-danger">Failed</span> : null}
              <button
                type="button"
                className="grid size-4 shrink-0 place-items-center rounded-full text-ink-3 transition-colors hover:bg-ink/10 hover:text-ink"
                aria-label={`Remove ${attachment.name}`}
                onClick={() => onRemove(attachment.localId)}
              >
                <X size={12} />
              </button>
            </div>
          ))}
        </div>
      ) : null}
    </div>
  )
}

function messageAttachmentFromComposer(attachment: ComposerAttachment): MessageAttachment {
  const file = composerAttachmentPreviewFile(attachment)
  return file ? { ...attachment, file } : attachment
}

export function ComposerAttachmentMenuRow({
  disabled,
  onChoose,
}: {
  disabled?: boolean
  onChoose: () => void
}) {
  return (
    <MenuRow disabled={disabled} onClick={onChoose}>
      <span className="flex items-center gap-2">
        <Paperclip size={13} />
        Attach files
      </span>
    </MenuRow>
  )
}
