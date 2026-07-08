import { useState } from 'react'
import { Eye, FileText, Image as ImageIcon } from 'lucide-react'
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
  const [show, setShow] = useState(false)
  const [unavailable, setUnavailable] = useState(false)
  const src = attachmentContentUrl(attachment, attachmentSessionId)
  if (!src || attachment.uploading) return <FileAttachmentPill attachment={attachment} />
  if (unavailable) return <UnavailableAttachmentPill attachment={attachment} />
  if (!show) return <ImageAttachmentPill attachment={attachment} onShow={() => setShow(true)} />
  return (
    <figure className="min-w-0 max-w-full">
      <img
        src={src}
        alt={attachment.name}
        loading="lazy"
        decoding="async"
        onError={() => setUnavailable(true)}
        className="block max-h-[360px] max-w-full rounded-[8px] bg-bg object-contain outline outline-1 -outline-offset-1 outline-black/10 dark:outline-white/10"
      />
      <figcaption className="mt-1 flex max-w-full items-center gap-1.5 text-xs text-ink-3">
        <span className="truncate text-ink-2">{attachment.name}</span>
        <span className="shrink-0">{attachmentStatus(attachment)}</span>
      </figcaption>
    </figure>
  )
}

function UnavailableAttachmentPill({ attachment }: { attachment: MessageAttachment }) {
  return (
    <span
      className="inline-flex max-w-full items-center gap-1.5 rounded-full bg-bg px-2.5 py-1 text-xs text-ink-2"
      title="Attachment file is no longer available on the server"
    >
      <ImageIcon size={13} className="shrink-0 text-ink-3" />
      <span className="max-w-[220px] truncate text-ink">{attachment.name}</span>
      <span className="shrink-0 text-ink-3">Unavailable</span>
    </span>
  )
}

function ImageAttachmentPill({
  attachment,
  onShow,
}: {
  attachment: MessageAttachment
  onShow: () => void
}) {
  return (
    <span
      className="inline-flex max-w-full items-center gap-1.5 rounded-full bg-bg px-2.5 py-1 text-xs text-ink-2"
      title={attachmentTitle(attachment)}
    >
      <ImageIcon size={13} className="shrink-0 text-ink-3" />
      <span className="max-w-[220px] truncate text-ink">{attachment.name}</span>
      <span className="shrink-0 text-ink-3">{formatAttachmentSize(attachment.size)}</span>
      <button
        type="button"
        aria-label={`Show ${attachment.name}`}
        title={`Show ${attachment.name}`}
        className="ml-0.5 inline-flex size-5 items-center justify-center rounded-full bg-surface-2 text-ink transition-colors hover:bg-border"
        onClick={onShow}
      >
        <Eye size={12} aria-hidden />
      </button>
    </span>
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
