import { Link } from '@tanstack/react-router'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { AnimatePresence, motion, useReducedMotion } from 'motion/react'
import { ArrowUpRight, Check, Archive as ArchiveIcon, CornerDownRight } from 'lucide-react'
import { useLayoutEffect, useRef, useState, type RefObject } from 'react'
import { createPortal } from 'react-dom'
import { MessageMarkdown } from '@/components/session/MessageMarkdown'
import { ComposerCard } from '@/components/session/Composer'
import { OverviewPanel, OVERVIEW_PANEL_WIDTH } from '@/components/session/OverviewPanel'
import { IconButton } from '@/components/ui/IconButton'
import { useToast } from '@/components/ui/toast'
import { markThreadSeen } from '@/lib/api/feed'
import {
  mutateSessionQueue,
  sessionQuery,
  setSessionArchived,
  uploadSessionAttachment,
} from '@/lib/api/sessions'
import type { FeedItem } from '@/lib/api/types'
import { relativeTime } from '@/lib/format/time'
import { invalidateSessionLists } from '@/lib/query/invalidate'
import { keys } from '@/lib/query/keys'
import { preparedSendMessage, type SendMessageHandler, type SendMessageOptions } from '@/lib/sendMessage'

const NO_TEXT = 'No text — open the thread to see tool activity.'

function snippet(text: string | undefined): string {
  if (!text) return NO_TEXT
  return text.replace(/\s+/g, ' ').trim()
}

export function FeedCard({
  item,
  expanded,
  onToggle,
}: {
  item: FeedItem
  expanded: boolean
  onToggle: () => void
}) {
  const cardRef = useRef<HTMLDivElement>(null)
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
      ref={cardRef}
      initial={reducedMotion ? false : { opacity: 0, y: 8 }}
      animate={{ opacity: 1, y: 0, marginBottom: 10 }}
      exit={reducedMotion ? { opacity: 0 } : { opacity: 0, height: 0, marginBottom: 0 }}
      transition={{ duration: 0.2, ease: 'easeOut' }}
      className={`overflow-hidden rounded-card bg-surface transition-colors duration-150 ${
        expanded ? '' : 'hover:bg-surface-2'
      }`}
    >
      <AnimatePresence>
        {expanded ? <FeedOverview anchorRef={cardRef} threadId={item.id} onSend={reply} /> : null}
      </AnimatePresence>
      <div
        role="button"
        tabIndex={0}
        aria-expanded={expanded}
        onClick={onToggle}
        onKeyDown={(e) => {
          if (e.key === 'Enter' || e.key === ' ') {
            e.preventDefault()
            onToggle()
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
          className="-mt-0.5 shrink-0 rounded-full p-1.5 text-ink-3 transition-colors duration-150 hover:bg-ink/10 hover:text-ink"
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
            <div className="bg-surface-2 px-3.5 py-3">
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
          className="hover:bg-ink/10!"
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
          className="hover:bg-danger/15!"
        >
          <ArchiveIcon size={15} />
        </IconButton>
      </div>
    </motion.div>
  )
}

function FeedOverview({
  anchorRef,
  threadId,
  onSend,
}: {
  anchorRef: RefObject<HTMLElement | null>
  threadId: string
  onSend: SendMessageHandler
}) {
  const session = useQuery(sessionQuery(threadId)).data
  const reducedMotion = useReducedMotion()
  const [rect, setRect] = useState<DOMRect | null>(null)

  useLayoutEffect(() => {
    const anchor = anchorRef.current
    if (!anchor) return
    const update = () => setRect(anchor.getBoundingClientRect())
    update()
    const observer = new ResizeObserver(update)
    observer.observe(anchor)
    window.addEventListener('scroll', update, true)
    window.addEventListener('resize', update)
    return () => {
      observer.disconnect()
      window.removeEventListener('scroll', update, true)
      window.removeEventListener('resize', update)
    }
  }, [anchorRef])

  if (!rect || !session) return null
  const gap = 12
  const panelWidth = OVERVIEW_PANEL_WIDTH + 16
  const fitsRight = rect.right + gap + panelWidth <= window.innerWidth
  const left = fitsRight ? rect.right + gap : rect.left - gap - panelWidth
  const from = reducedMotion ? 0 : (fitsRight ? -1 : 1) * 16
  return createPortal(
    <motion.div
      style={{ position: 'fixed', top: rect.top + rect.height / 2, left }}
      initial={{ opacity: 0, x: from, y: '-50%' }}
      animate={{ opacity: 1, x: 0, y: '-50%' }}
      exit={{ opacity: 0, x: from, y: '-50%' }}
      transition={{ duration: 0.18, ease: 'easeOut' }}
      className="z-drawer"
    >
      <OverviewPanel
        session={session}
        subagents={[]}
        spawnedThreads={[]}
        working={session.status === 'running'}
        onSend={onSend}
      />
    </motion.div>,
    document.body,
  )
}
