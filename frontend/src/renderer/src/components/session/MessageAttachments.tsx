import { useLayoutEffect, useState } from 'react'
import { File as FileIcon, Image as ImageIcon, LoaderCircle } from 'lucide-react'
import { Modal } from '@/components/ui/Modal'
import { sessionAttachmentUrl } from '@/lib/api/sessions'

const IMAGE_MIME_TYPES = new Set([
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
  if (!attachments.length) {
    return null
  }
  return (
    <div className="mb-1 flex max-w-[84%] flex-col items-end gap-2">
      {attachments.map((attachment, index) => (
        <AttachmentTile
          key={attachmentKey(attachment, index)}
          attachment={attachment}
          attachmentSessionId={attachmentSessionId}
        />
      ))}
    </div>
  )
}

export function AttachmentTile({
  attachment,
  attachmentSessionId,
  variant = 'message',
}: {
  attachment: MessageAttachment
  attachmentSessionId?: string
  variant?: 'composer' | 'message'
}) {
  const [open, setOpen] = useState(false)
  const [failedSrc, setFailedSrc] = useState('')
  const isImage = isImageAttachment(attachment)
  const objectUrl = useObjectUrl(isImage ? attachment.file : undefined)
  const src = objectUrl || attachmentContentUrl(attachment, attachmentSessionId)
  const available = isImage && Boolean(src) && src !== failedSrc
  const status = attachmentStatus(attachment)
  if (!available && variant === 'message') {
    return <FileAttachmentPill attachment={attachment} />
  }
  return (
    <>
      <button
        type="button"
        aria-label={`${available ? 'Open ' : ''}${attachment.name}`}
        title={attachmentTitle(attachment)}
        disabled={!available}
        className={`relative block max-w-full shrink-0 overflow-hidden rounded-[10px] bg-bg text-left enabled:cursor-zoom-in focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-primary ${variant === 'message' ? 'size-20' : available ? 'size-30' : 'h-30 w-40'}`}
        onClick={() => setOpen(true)}
      >
        {available ? (
          <img
            src={src}
            alt={attachment.name}
            loading="lazy"
            onError={() => setFailedSrc(src)}
            className="block size-full rounded-[10px] object-cover outline outline-1 -outline-offset-1 outline-black/10 dark:outline-white/10"
          />
        ) : (
          <div className="flex h-full flex-col">
            <div className="grid min-h-0 flex-1 place-items-center text-ink-3">
              <FileIcon size={20} aria-hidden />
            </div>
            <div className="flex min-w-0 items-center gap-1.5 px-2.5 py-2 text-xs text-ink">
              <FileIcon size={13} className="shrink-0" aria-hidden />
              <span className="truncate">{attachment.name}</span>
            </div>
          </div>
        )}
        {status ? (
          <span role="status" className={`absolute top-2 left-2 flex items-center gap-1 rounded-md bg-bg/95 px-1.5 py-1 text-[10px] ${attachment.error ? 'text-danger' : 'text-ink-2'}`}>
            {attachment.uploading ? <LoaderCircle size={11} className="animate-spin" aria-hidden /> : null}
            {status}
          </span>
        ) : null}
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
          <span className="shrink-0 tabular-nums">{attachmentStatus(attachment) || formatAttachmentSize(attachment.size)}</span>
        </figcaption>
      </figure>
    </Modal>
  )
}

function FileAttachmentPill({ attachment }: { attachment: MessageAttachment }) {
  const Icon = isImageAttachment(attachment) ? ImageIcon : FileIcon
  const status = attachmentStatus(attachment)
  return (
    <span
      className="inline-flex max-w-full items-center gap-1.5 rounded-full bg-surface px-2.5 py-1.5 text-sm text-ink-2 ring-1 ring-inset ring-border/70"
      title={attachmentTitle(attachment)}
    >
      <Icon size={13} className="shrink-0 text-ink-3" aria-hidden />
      <span className="min-w-0 truncate text-ink">{attachment.name}</span>
      {status ? (
        <span role="status" className={`shrink-0 ${attachment.error ? 'text-danger' : 'text-ink-3'}`}>{status}</span>
      ) : null}
    </span>
  )
}

function attachmentStatus(attachment: MessageAttachment): string {
  if (attachment.error) {
    return 'Failed'
  }
  return attachment.uploading ? 'Uploading' : ''
}

function attachmentContentUrl(attachment: MessageAttachment, attachmentSessionId?: string): string {
  if (attachment.uri && !attachment.uri.startsWith('file:')) return attachment.uri
  if (attachmentSessionId && attachment.id) return sessionAttachmentUrl(attachmentSessionId, attachment.id)
  return ''
}

function isImageAttachment(attachment: MessageAttachment): boolean {
  const mime = attachment.mime_type?.split(';', 1)[0]?.trim().toLowerCase() ?? ''
  return IMAGE_MIME_TYPES.has(mime) || /\.(avif|bmp|gif|heic|heif|jpe?g|png|tiff?|webp)$/i.test(attachment.name)
}

function formatAttachmentSize(size?: number): string {
  if (!size) return ''
  if (size < 1024) return `${size} B`
  if (size < 1024 * 1024) return `${Math.round(size / 1024)} KB`
  return `${(size / (1024 * 1024)).toFixed(1)} MB`
}

function attachmentTitle(attachment: MessageAttachment): string {
  return attachment.error ?? attachment.uri ?? attachment.name
}

function attachmentKey(attachment: MessageAttachment, index: number): string {
  return attachment.id ?? `${attachment.name}-${index}`
}
