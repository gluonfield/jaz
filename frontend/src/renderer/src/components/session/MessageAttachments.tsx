import { useState } from 'react'
import { Eye, FileText, Image as ImageIcon, ImageOff, LoaderCircle } from 'lucide-react'
import { Modal } from '@/components/ui/Modal'
import { sessionAttachmentUrl } from '@/lib/api/sessions'

const RENDERABLE_IMAGE_MIME_TYPES = new Set([
  'image/avif',
  'image/bmp',
  'image/gif',
  'image/heic',
  'image/heif',
  'image/jpeg',
  'image/png',
  'image/tiff',
  'image/webp',
])

export interface MessageAttachment {
  id?: string
  name: string
  uri?: string
  mime_type?: string
  size?: number
  uploading?: boolean
}

export function MessageAttachments({
  attachments,
  attachmentSessionId,
}: {
  attachments: MessageAttachment[]
  attachmentSessionId?: string
}) {
  if (!attachments.length) return null
  const images = attachments.filter(isImageAttachment)
  const files = attachments.filter((attachment) => !isImageAttachment(attachment))
  return (
    <div className="mt-2 flex max-w-full flex-col gap-2">
      {images.length ? (
        <div className="flex max-w-full flex-wrap gap-2">
          {images.map((attachment, index) => (
            <ImageAttachment
              key={attachment.id ?? `${attachment.name}-${index}`}
              attachment={attachment}
              attachmentSessionId={attachmentSessionId}
            />
          ))}
        </div>
      ) : null}
      {files.length ? (
        <div className="flex flex-wrap gap-1">
          {files.map((attachment, index) => (
            <FileAttachmentPill key={attachment.id ?? `${attachment.name}-${index}`} attachment={attachment} />
          ))}
        </div>
      ) : null}
    </div>
  )
}

function ImageAttachment({
  attachment,
  attachmentSessionId,
}: {
  attachment: MessageAttachment
  attachmentSessionId?: string
}) {
  const [open, setOpen] = useState(false)
  const [unavailable, setUnavailable] = useState(false)
  const src = attachmentContentUrl(attachment, attachmentSessionId)
  if (attachment.uploading) return <ImageAttachmentPreview attachment={attachment} status="Uploading" uploading />
  if (!src || unavailable) return <ImageAttachmentPreview attachment={attachment} status="Unavailable" unavailable />
  return (
    <>
      <ImageAttachmentPreview attachment={attachment} onOpen={() => setOpen(true)} />
      <ImageAttachmentModal
        attachment={attachment}
        src={src}
        open={open}
        onClose={() => setOpen(false)}
        onError={() => setUnavailable(true)}
      />
    </>
  )
}

function ImageAttachmentPreview({
  attachment,
  onOpen,
  status = formatAttachmentSize(attachment.size),
  unavailable = false,
  uploading = false,
}: {
  attachment: MessageAttachment
  onOpen?: () => void
  status?: string
  unavailable?: boolean
  uploading?: boolean
}) {
  const content = (
    <>
      <div className="relative grid aspect-[4/3] place-items-center overflow-hidden bg-surface-2">
        <div className="relative grid size-7 place-items-center rounded-[6px] bg-bg/70 text-ink-3 shadow-sm ring-1 ring-border/70">
          {uploading ? (
            <LoaderCircle size={15} className="animate-spin" aria-hidden />
          ) : unavailable ? (
            <ImageOff size={15} aria-hidden />
          ) : (
            <ImageIcon size={15} aria-hidden />
          )}
        </div>
        {onOpen ? (
          <span className="absolute top-1 right-1 grid size-5 place-items-center rounded-full bg-bg/90 text-ink shadow-sm ring-1 ring-border/70">
            <Eye size={10} aria-hidden />
          </span>
        ) : null}
      </div>
      <div className="flex min-w-0 items-center gap-1 px-1.5 py-1 text-[10px]">
        <ImageIcon size={11} className="shrink-0 text-ink-3" aria-hidden />
        <span className="min-w-0 flex-1 truncate text-ink">{attachment.name}</span>
        {status ? <span className="shrink-0 text-ink-3">{status}</span> : null}
      </div>
    </>
  )

  if (!onOpen) {
    return (
      <div
        className="w-24 max-w-full overflow-hidden rounded-[8px] bg-bg text-left shadow-sm ring-1 ring-border/70"
        title={unavailable ? 'Attachment file is no longer available on the server' : attachmentTitle(attachment)}
      >
        {content}
      </div>
    )
  }

  return (
    <button
      type="button"
      aria-label={`Open ${attachment.name}`}
      title={attachmentTitle(attachment)}
      className="w-24 max-w-full cursor-pointer overflow-hidden rounded-[8px] bg-bg text-left shadow-sm ring-1 ring-border/70 transition-[background-color,transform] duration-150 hover:bg-surface-2 active:scale-[0.96]"
      onClick={onOpen}
    >
      {content}
    </button>
  )
}

function ImageAttachmentModal({
  attachment,
  src,
  open,
  onClose,
  onError,
}: {
  attachment: MessageAttachment
  src: string
  open: boolean
  onClose: () => void
  onError: () => void
}) {
  return (
    <Modal open={open} onClose={onClose} title={attachment.name} size="xl" chromeless>
      <figure className="flex max-h-[calc(100dvh-3rem)] min-h-0 flex-col bg-black/90">
        <div className="grid min-h-0 flex-1 place-items-center p-3 sm:p-4">
          <img
            src={src}
            alt={attachment.name}
            onError={onError}
            className="max-h-[calc(100dvh-7rem)] max-w-full rounded-[8px] object-contain outline outline-1 -outline-offset-1 outline-white/10"
          />
        </div>
        <figcaption className="flex min-h-9 items-center gap-2 px-3 py-2 text-[12px] text-white/70 sm:px-4">
          <span className="min-w-0 flex-1 truncate text-white/85">{attachment.name}</span>
          <span className="shrink-0 tabular-nums">{attachmentStatus(attachment)}</span>
        </figcaption>
      </figure>
    </Modal>
  )
}

function FileAttachmentPill({ attachment }: { attachment: MessageAttachment }) {
  return (
    <span
      className="inline-flex max-w-full items-center gap-1.5 rounded-full bg-bg px-2.5 py-1 text-xs text-ink-2"
      title={attachmentTitle(attachment)}
    >
      <FileText size={13} className="shrink-0 text-ink-3" />
      <span className="max-w-[220px] truncate text-ink">{attachment.name}</span>
      <span className="shrink-0 text-ink-3">{attachmentStatus(attachment)}</span>
    </span>
  )
}

function attachmentStatus(attachment: MessageAttachment): string {
  return attachment.uploading ? 'Uploading' : formatAttachmentSize(attachment.size)
}

function attachmentContentUrl(attachment: MessageAttachment, attachmentSessionId?: string): string {
  if (attachment.uri && !attachment.uri.startsWith('file:')) return attachment.uri
  if (attachmentSessionId && attachment.id) return sessionAttachmentUrl(attachmentSessionId, attachment.id)
  return ''
}

function isImageAttachment(attachment: MessageAttachment): boolean {
  const mime = attachment.mime_type?.split(';', 1)[0]?.trim().toLowerCase() ?? ''
  return RENDERABLE_IMAGE_MIME_TYPES.has(mime) || /\.(avif|bmp|gif|heic|heif|jpe?g|png|tiff?|webp)$/i.test(attachment.name)
}

function formatAttachmentSize(size?: number): string {
  if (!size) return ''
  if (size < 1024) return `${size} B`
  if (size < 1024 * 1024) return `${Math.round(size / 1024)} KB`
  return `${(size / (1024 * 1024)).toFixed(1)} MB`
}

function attachmentTitle(attachment: MessageAttachment): string {
  return attachment.uri ?? attachment.name
}
