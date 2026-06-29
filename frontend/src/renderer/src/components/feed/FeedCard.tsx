import { Link, useNavigate } from '@tanstack/react-router'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { AnimatePresence, motion, useReducedMotion } from 'motion/react'
import { ArrowUpRight, Check, Archive as ArchiveIcon, CornerDownRight } from 'lucide-react'
import { useState } from 'react'
import { MessageMarkdown } from '@/components/session/MessageMarkdown'
import { ComposerCard } from '@/components/session/Composer'
import { markThreadSeen } from '@/lib/api/feed'
import { setSessionArchived } from '@/lib/api/sessions'
import type { FeedItem } from '@/lib/api/types'
import { relativeTime } from '@/lib/format/time'
import { setPendingMessage } from '@/lib/pendingMessage'
import { invalidateSessionLists } from '@/lib/query/invalidate'
import { keys } from '@/lib/query/keys'
import type { SendMessageOptions } from '@/lib/sendMessage'

const NO_TEXT = 'No text — open the thread to see tool activity.'

function snippet(text: string | undefined): string {
  if (!text) return NO_TEXT
  return text.replace(/\s+/g, ' ').trim()
}

export function FeedCard({ item }: { item: FeedItem }) {
  const [expanded, setExpanded] = useState(false)
  const queryClient = useQueryClient()
  const navigate = useNavigate()
  const reducedMotion = useReducedMotion()

  // Optimistically drop the card from the cached feed, then reconcile.
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

  // Replying reuses the proven thread send pipeline: hand the draft to the
  // thread view and open it. The server marks the thread seen on send, so it
  // leaves the feed on its own.
  const reply = (text: string, options: SendMessageOptions = {}) => {
    const trimmed = text.trim()
    if (!trimmed) return
    setPendingMessage(item.id, {
      text,
      planRequested: Boolean(options.planRequested),
      goalRequested: Boolean(options.goalRequested),
      files: options.files ?? [],
    })
    removeFromFeed()
    navigate({ to: '/sessions/$sessionId', params: { sessionId: item.id } })
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
      <div className="flex items-start gap-3 p-3.5">
        <button
          type="button"
          onClick={() => setExpanded((open) => !open)}
          className="min-w-0 flex-1 cursor-pointer text-left"
          aria-expanded={expanded}
        >
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
        </button>
        <div className="flex shrink-0 items-center gap-0.5 text-ink-3">
          <button
            type="button"
            disabled={busy}
            onClick={() => done.mutate()}
            aria-label="Mark done"
            title="Mark done"
            className="rounded-full p-1.5 transition-colors duration-150 hover:bg-surface-2 hover:text-ink disabled:opacity-50"
          >
            <Check size={16} />
          </button>
          <button
            type="button"
            disabled={busy}
            onClick={() => archive.mutate()}
            aria-label="Archive"
            title="Archive"
            className="rounded-full p-1.5 transition-colors duration-150 hover:bg-surface-2 hover:text-ink disabled:opacity-50"
          >
            <ArchiveIcon size={16} />
          </button>
          <Link
            to="/sessions/$sessionId"
            params={{ sessionId: item.id }}
            aria-label="Open thread"
            title="Open thread"
            className="rounded-full p-1.5 transition-colors duration-150 hover:bg-surface-2 hover:text-ink"
          >
            <ArrowUpRight size={16} />
          </Link>
        </div>
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
            <div className="border-t border-border px-3.5 pb-3.5 pt-3">
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
    </motion.div>
  )
}
