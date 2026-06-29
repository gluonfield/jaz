import { Link } from '@tanstack/react-router'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { AnimatePresence, motion, useReducedMotion } from 'motion/react'
import { ArrowUpRight, Check, Archive as ArchiveIcon, CornerDownRight } from 'lucide-react'
import { useState } from 'react'
import { MessageMarkdown } from '@/components/session/MessageMarkdown'
import { ComposerCard } from '@/components/session/Composer'
import { IconButton } from '@/components/ui/IconButton'
import { useToast } from '@/components/ui/toast'
import { markThreadSeen } from '@/lib/api/feed'
import { mutateSessionQueue, setSessionArchived, uploadSessionAttachment } from '@/lib/api/sessions'
import type { FeedItem } from '@/lib/api/types'
import { relativeTime } from '@/lib/format/time'
import { invalidateSessionLists } from '@/lib/query/invalidate'
import { keys } from '@/lib/query/keys'
import { preparedSendMessage, type SendMessageOptions } from '@/lib/sendMessage'

const NO_TEXT = 'No text — open the thread to see tool activity.'

function snippet(text: string | undefined): string {
  if (!text) return NO_TEXT
  return text.replace(/\s+/g, ' ').trim()
}

export function FeedCard({ item }: { item: FeedItem }) {
  const [expanded, setExpanded] = useState(false)
  const queryClient = useQueryClient()
  const toast = useToast()
  const reducedMotion = useReducedMotion()

  const removeFromFeed = () => {
    queryClient.setQueryData<FeedItem[]>(keys.feed, (prev) =>
      (prev ?? []).filter((entry) => entry.id !== item.id),
    )
  }

  const done = useMutation({
    mutationFn: () => markThreadSeen(item.id),
    onMutate: removeFromFeed,
    onSettled: () => queryClient.invalidateQueries({ queryKey: keys.feed }),
  })

  const archive = useMutation({
    mutationFn: () => setSessionArchived(item.id, true),
    onMutate: removeFromFeed,
    onSettled: () => invalidateSessionLists(queryClient, { archived: true }),
  })

  // Send straight to the thread's queue (the backend starts the turn when idle)
  // and let the card animate out — no navigation. Mark seen so the poll doesn't
  // bounce it back before the agent's next reply arrives.
  const reply = async (text: string, options: SendMessageOptions = {}) => {
    if (!text.trim()) return
    removeFromFeed()
    try {
      const uploaded = options.files?.length
        ? await Promise.all(options.files.map((file) => uploadSessionAttachment(item.id, file)))
        : []
      const prepared = preparedSendMessage(options, uploaded)
      await markThreadSeen(item.id)
      await mutateSessionQueue(item.id, {
        op: 'append',
        message: {
          text,
          contexts: prepared.contexts,
          attachment_ids: prepared.attachmentIds,
          plan_requested: options.planRequested,
          goal_requested: options.goalRequested,
        },
      })
    } catch (error) {
      toast(`Couldn't send reply: ${(error as Error).message}`, 'danger')
      queryClient.invalidateQueries({ queryKey: keys.feed })
    }
  }

  const title = item.title?.trim() || item.slug
  const busy = done.isPending || archive.isPending

  return (
    <motion.div
      initial={reducedMotion ? false : { opacity: 0, y: 8 }}
      animate={{ opacity: 1, y: 0 }}
      exit={reducedMotion ? { opacity: 0 } : { opacity: 0, scale: 0.98 }}
      transition={{ duration: 0.18, ease: 'easeOut' }}
      className={`overflow-hidden rounded-card border border-border bg-surface transition-colors duration-150 ${
        expanded ? '' : 'hover:bg-surface-2'
      }`}
    >
      <div
        role="button"
        tabIndex={0}
        aria-expanded={expanded}
        onClick={() => setExpanded((open) => !open)}
        onKeyDown={(e) => {
          if (e.key === 'Enter' || e.key === ' ') {
            e.preventDefault()
            setExpanded((open) => !open)
          }
        }}
        className="flex cursor-pointer items-start gap-3 px-3.5 pt-3.5 pb-2 text-left"
      >
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            {item.parent_id ? (
              <CornerDownRight size={13} className="shrink-0 text-ink-3" aria-hidden />
            ) : null}
            <span className="truncate text-[13px] font-medium text-ink">{title}</span>
            {item.status === 'running' ? (
              <span className="size-1.5 shrink-0 rounded-full bg-primary" aria-label="running" />
            ) : null}
            <span className="ml-auto shrink-0 text-[12px] tabular-nums text-ink-3">
              {relativeTime(item.last_message.created_at)}
            </span>
          </div>
          {expanded ? null : (
            <p className="mt-1 line-clamp-2 text-[13px] leading-relaxed text-ink-2">
              {snippet(item.last_message.text)}
            </p>
          )}
        </div>
        <Link
          to="/sessions/$sessionId"
          params={{ sessionId: item.id }}
          aria-label="Open thread"
          title="Open thread"
          onClick={(e) => e.stopPropagation()}
          className="-mt-0.5 shrink-0 rounded-full p-1.5 text-ink-3 transition-colors duration-150 hover:bg-surface-2 hover:text-ink"
        >
          <ArrowUpRight size={16} />
        </Link>
      </div>

      <AnimatePresence initial={false}>
        {expanded ? (
          <motion.div
            initial={{ height: 0, opacity: 0 }}
            animate={{ height: 'auto', opacity: 1 }}
            exit={{ height: 0, opacity: 0 }}
            transition={{ duration: 0.18, ease: 'easeOut' }}
            className="overflow-hidden"
          >
            <div className="border-t border-border bg-surface-2 px-3.5 py-3">
              <div className="max-h-[42vh] overflow-y-auto text-[13px] leading-relaxed text-ink [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
                {item.last_message.text ? (
                  <MessageMarkdown text={item.last_message.text} />
                ) : (
                  <p className="text-ink-3">{NO_TEXT}</p>
                )}
              </div>
              <div className="mt-3">
                <ComposerCard
                  streaming={false}
                  placeholder="Reply…"
                  draftStorageKey={`feed:${item.id}`}
                  onSend={reply}
                />
              </div>
            </div>
          </motion.div>
        ) : null}
      </AnimatePresence>

      <div
        className={`flex items-center justify-end gap-1 px-3 pb-2.5 pt-1.5 ${
          expanded ? 'bg-surface-2' : ''
        }`}
      >
        <IconButton
          size="sm"
          disabled={busy}
          onClick={() => done.mutate()}
          aria-label="Mark done"
          title="Mark done"
        >
          <Check size={15} />
        </IconButton>
        <IconButton
          size="sm"
          variant="danger"
          disabled={busy}
          onClick={() => archive.mutate()}
          aria-label="Archive"
          title="Archive"
        >
          <ArchiveIcon size={15} />
        </IconButton>
      </div>
    </motion.div>
  )
}
