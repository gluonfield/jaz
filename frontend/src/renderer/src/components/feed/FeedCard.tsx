import { Link, useNavigate } from '@tanstack/react-router'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { motion, useReducedMotion } from 'motion/react'
import { ArrowUpRight, Check, Archive as ArchiveIcon, CornerDownRight } from 'lucide-react'
import { useState } from 'react'
import { Button } from '@/components/ui/Button'
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
      layout={!reducedMotion}
      initial={reducedMotion ? false : { opacity: 0, y: 8 }}
      animate={{ opacity: 1, y: 0 }}
      exit={reducedMotion ? undefined : { opacity: 0, scale: 0.98 }}
      transition={{ type: 'spring', stiffness: 420, damping: 36 }}
      className={`group rounded-2xl border border-border bg-surface-1 transition-colors duration-150 ${
        expanded ? 'ring-1 ring-border' : 'hover:bg-surface-2'
      }`}
    >
      <div className="flex items-start gap-3 p-4">
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
            <span className="truncate text-[14px] font-medium text-ink">{title}</span>
            {item.status === 'running' ? (
              <span className="size-1.5 shrink-0 rounded-full bg-primary" aria-label="running" />
            ) : null}
            <span className="ml-auto shrink-0 text-[12px] tabular-nums text-ink-3">
              {relativeTime(item.last_message.created_at)}
            </span>
          </div>
          <p
            className={`mt-1 text-[13px] leading-relaxed text-ink-2 ${expanded ? '' : 'line-clamp-2'}`}
          >
            {expanded ? null : snippet(item.last_message.text)}
          </p>
        </button>
        <Link
          to="/sessions/$sessionId"
          params={{ sessionId: item.id }}
          aria-label="Open thread"
          className="shrink-0 rounded-full p-1.5 text-ink-3 transition-colors duration-150 hover:bg-surface-2 hover:text-ink"
        >
          <ArrowUpRight size={16} />
        </Link>
      </div>

      {expanded ? (
        <div className="border-t border-border px-4 pb-4 pt-3">
          <div className="max-h-[42vh] overflow-y-auto pr-1 text-[13px] leading-relaxed text-ink">
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
          <div className="mt-2 flex items-center justify-end gap-1.5">
            <Button variant="ghost" size="sm" disabled={busy} onClick={() => done.mutate()}>
              <Check size={14} />
              Done
            </Button>
            <Button variant="ghost" size="sm" disabled={busy} onClick={() => archive.mutate()}>
              <ArchiveIcon size={14} />
              Archive
            </Button>
          </div>
        </div>
      ) : null}
    </motion.div>
  )
}
