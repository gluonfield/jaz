import { FileText } from 'lucide-react'
import type { LiveAttachment } from './useLiveSessionSend'

export function LiveAttachmentList({ attachments }: { attachments: LiveAttachment[] }) {
  if (!attachments.length) return null
  return (
    <div className="mt-2 flex flex-wrap gap-1">
      {attachments.map((attachment, index) => (
        <span
          key={attachment.id ?? `${attachment.name}-${index}`}
          className="inline-flex max-w-full items-center gap-1.5 rounded-full bg-bg px-2.5 py-1 text-xs text-ink-2"
        >
          <FileText size={13} className="shrink-0 text-primary" />
          <span className="max-w-[220px] truncate text-ink">{attachment.name}</span>
          <span className="shrink-0 text-ink-3">
            {attachment.uploading ? 'Uploading' : formatAttachmentSize(attachment.size)}
          </span>
        </span>
      ))}
    </div>
  )
}

function formatAttachmentSize(size?: number): string {
  if (!size) return ''
  if (size < 1024) return `${size} B`
  if (size < 1024 * 1024) return `${Math.round(size / 1024)} KB`
  return `${(size / (1024 * 1024)).toFixed(1)} MB`
}
