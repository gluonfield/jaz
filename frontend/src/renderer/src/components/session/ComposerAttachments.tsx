import { FileText, LoaderCircle, Paperclip, X } from 'lucide-react'
import type { RefObject } from 'react'
import { MenuRow } from '@/components/ui/Popover'
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
  onRemove,
}: {
  attachments: ComposerAttachment[]
  onRemove: (localId: string) => void
}) {
  if (attachments.length === 0) return null
  return (
    <div className="flex flex-wrap gap-1 px-1.5 pt-0.5">
      {attachments.map((attachment) => (
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
