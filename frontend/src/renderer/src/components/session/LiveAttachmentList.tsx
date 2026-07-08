import type { LiveAttachment } from './useLiveSessionSend'
import { MessageAttachments } from './MessageAttachments'

export function LiveAttachmentList({
  attachments,
  attachmentSessionId,
}: {
  attachments: LiveAttachment[]
  attachmentSessionId?: string
}) {
  return <MessageAttachments attachments={attachments} attachmentSessionId={attachmentSessionId} />
}
