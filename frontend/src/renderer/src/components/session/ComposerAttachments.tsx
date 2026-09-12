import { Paperclip, X } from 'lucide-react'
import type { RefObject } from 'react'
import { MenuRow } from '@/components/ui/Popover'
import { AttachmentTile } from '@/components/session/MessageAttachments'
import { composerAttachmentPreviewFile, type ComposerAttachment } from '@/components/session/composerAttachmentTypes'

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
  if (attachments.length === 0) {
    return null
  }
  return (
    <div className="flex max-w-full flex-wrap gap-3 px-1.5 pt-0.5 pb-1">
      {attachments.map((attachment) => (
        <div key={attachment.localId} className="group/attachment relative max-w-full">
          <AttachmentTile
            attachment={{ ...attachment, file: composerAttachmentPreviewFile(attachment) }}
            attachmentSessionId={attachmentSessionId}
            variant="composer"
          />
          <button
            type="button"
            className="absolute top-0 right-0 z-10 grid size-10 place-items-center rounded-tr-[10px] text-ink-2 transition-opacity duration-150 [@media(hover:hover)]:opacity-0 group-hover/attachment:opacity-100 group-focus-within/attachment:opacity-100 focus-visible:outline-2 focus-visible:outline-primary"
            aria-label={`Remove ${attachment.name}`}
            title={`Remove ${attachment.name}`}
            onClick={() => onRemove(attachment.localId)}
          >
            <span className="grid size-6 place-items-center rounded-full bg-bg/95 shadow-sm ring-1 ring-border/70 backdrop-blur-sm transition-[background-color,color,transform] duration-150 hover:bg-surface-2 hover:text-ink active:scale-[0.96]">
              <X size={12} />
            </span>
          </button>
        </div>
      ))}
    </div>
  )
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
