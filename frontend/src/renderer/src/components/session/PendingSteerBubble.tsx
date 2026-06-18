import { Paperclip } from 'lucide-react'
import { motion } from 'motion/react'
import type { QueuedMessage } from '@/lib/api/types'
import { MentionText } from './mentions'

export function PendingSteerBubble({ prompt }: { prompt: QueuedMessage }) {
  const attachmentCount = prompt.attachment_ids?.length ?? 0
  return (
    <motion.div
      className="flex justify-end"
      initial={{ opacity: 0, y: 8 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ type: 'spring', stiffness: 380, damping: 30 }}
    >
      <div className="min-w-0 max-w-[84%] rounded-card border border-primary/20 bg-surface px-3.5 py-2.5 text-sm whitespace-pre-wrap [overflow-wrap:break-word] shadow-sm select-text">
        <div className="mb-1 text-[11px] font-medium tracking-normal text-primary">Steering...</div>
        <MentionText text={prompt.text} />
        {attachmentCount ? (
          <div className="mt-2 flex flex-wrap gap-1">
            <span className="inline-flex max-w-full items-center gap-1.5 rounded-full bg-bg px-2.5 py-1 text-xs text-ink-2">
              <Paperclip size={13} className="shrink-0 text-primary" />
              <span className="text-ink">
                {attachmentCount} attachment{attachmentCount === 1 ? '' : 's'}
              </span>
            </span>
          </div>
        ) : null}
      </div>
    </motion.div>
  )
}
