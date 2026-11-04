import { Paperclip, X } from 'lucide-react'
import { useState, type RefObject } from 'react'
import { MenuRow, Popover } from '@/components/ui/Popover'
import { AttachmentStatusBadge, AttachmentTile, FileAttachmentPill } from '@/components/session/MessageAttachments'
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

const PREVIEW_LIMIT = 4

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
  const overflow = attachments.slice(PREVIEW_LIMIT)
  return (
    <div className="flex max-w-full flex-wrap gap-3 px-1.5 pt-0.5 pb-1">
      {attachments.slice(0, PREVIEW_LIMIT).map((attachment) => (
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
      {overflow.length > 0 ? <AttachmentOverflow attachments={overflow} onRemove={onRemove} /> : null}
    </div>
  )
}

function AttachmentOverflow({
  attachments,
  onRemove,
}: {
  attachments: ComposerAttachment[]
  onRemove: (localId: string) => void
}) {
  const [open, setOpen] = useState(false)
  const flagged = attachments.find((attachment) => attachment.error) ?? attachments.find((attachment) => attachment.uploading)
  return (
    <Popover
      open={open}
      onClose={() => setOpen(false)}
      trigger={
        <button
          type="button"
          aria-expanded={open}
          aria-label={`${attachments.length} more ${attachments.length === 1 ? 'attachment' : 'attachments'}`}
          className="relative grid size-30 place-items-center rounded-[10px] bg-bg text-lg text-ink-2 tabular-nums transition-colors duration-150 hover:bg-surface-2 hover:text-ink focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-primary"
          onClick={() => setOpen(!open)}
        >
          +{attachments.length}
          {flagged ? <AttachmentStatusBadge attachment={flagged} /> : null}
        </button>
      }
    >
      <div className="flex max-h-64 max-w-80 flex-col items-start gap-1.5 overflow-y-auto">
        {attachments.map((attachment, index) => (
          <FileAttachmentPill
            key={attachment.localId}
            attachment={attachment}
            autoFocus={index === 0}
            onRemove={() => onRemove(attachment.localId)}
          />
        ))}
      </div>
    </Popover>
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
