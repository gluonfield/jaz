import { useLayoutEffect, useState } from 'react'
import { FileText, Image as ImageIcon, ImageOff, LoaderCircle } from 'lucide-react'
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
  file?: File
  uploading?: boolean
  error?: string
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
        <div className="flex max-w-full flex-col items-start gap-2">
          {images.map((attachment, index) => (
            <ImageAttachmentTile
              key={attachmentKey(attachment, index)}
              attachment={attachment}
              attachmentSessionId={attachmentSessionId}
            />
          ))}
        </div>
      ) : null}
      {files.length ? (
        <div className="flex flex-wrap gap-1">
          {files.map((attachment, index) => (
            <FileAttachmentPill key={attachmentKey(attachment, index)} attachment={attachment} />
          ))}
        </div>
      ) : null}
    </div>
  )
}

export function ImageAttachmentTile({
  attachment,
  attachmentSessionId,
  compact = false,
}: {
  attachment: MessageAttachment
  attachmentSessionId?: string
  compact?: boolean
}) {
  const [open, setOpen] = useState(false)
  const [failedSrc, setFailedSrc] = useState('')
  const objectUrl = useObjectUrl(attachment.file)
  const src = objectUrl || attachmentContentUrl(attachment, attachmentSessionId)
  const available = Boolean(src) && src !== failedSrc
  const status = available || attachment.uploading ? attachmentStatus(attachment) : 'Unavailable'
  return (
    <>
      <button
        type="button"
        aria-label={`${available ? 'Open' : status} ${attachment.name}`}
        title={attachmentTitle(attachment)}
        disabled={!available}
        className={`max-w-full overflow-hidden rounded-[8px] bg-bg text-left shadow-sm ring-1 ring-border/70 enabled:cursor-zoom-in ${compact || !available ? 'w-24' : 'w-fit'}`}
        onClick={() => setOpen(true)}
      >
        {available ? (
          <img
            src={src}
            alt={attachment.name}
            loading="lazy"
            onError={() => setFailedSrc(src)}
            className={`block max-w-full object-contain outline outline-1 -outline-offset-1 outline-black/10 dark:outline-white/10 ${compact ? 'h-18 w-24' : 'max-h-[32rem] h-auto w-auto'}`}
          />
        ) : (
          <div className="grid aspect-[4/3] place-items-center bg-surface-2 text-ink-3">
            {attachment.uploading ? (
              <LoaderCircle size={18} className="animate-spin" aria-hidden />
            ) : (
              <ImageOff size={18} aria-hidden />
            )}
          </div>
        )}
        <div className="flex min-w-0 items-center gap-1 px-1.5 py-1 text-[10px]">
          <ImageIcon size={11} className="shrink-0 text-ink-3" aria-hidden />
          <span className="min-w-0 flex-1 truncate text-ink">{attachment.name}</span>
          {status ? <span className="shrink-0 text-ink-3">{status}</span> : null}
        </div>
      </button>
      {available ? (
        <ImageAttachmentModal
          attachment={attachment}
          src={src}
          open={open}
          onClose={() => setOpen(false)}
          onError={() => setFailedSrc(src)}
        />
      ) : null}
    </>
  )
}

function useObjectUrl(file?: File): string {
  const [url, setUrl] = useState('')
  useLayoutEffect(() => {
    if (!file) {
      setUrl('')
      return
    }
    const next = URL.createObjectURL(file)
    setUrl(next)
    return () => URL.revokeObjectURL(next)
  }, [file])
  return url
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
  if (attachment.error) return 'Failed'
  return attachment.uploading ? 'Uploading' : formatAttachmentSize(attachment.size)
}

function attachmentContentUrl(attachment: MessageAttachment, attachmentSessionId?: string): string {
  if (attachment.uri && !attachment.uri.startsWith('file:')) return attachment.uri
  if (attachmentSessionId && attachment.id) return sessionAttachmentUrl(attachmentSessionId, attachment.id)
  return ''
}

export function isImageAttachment(attachment: MessageAttachment): boolean {
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

function attachmentKey(attachment: MessageAttachment, index: number): string {
  return attachment.id ?? `${attachment.name}-${index}`
}
