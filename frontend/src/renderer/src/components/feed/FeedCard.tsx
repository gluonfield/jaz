import { Link, useNavigate } from '@tanstack/react-router'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { AnimatePresence, motion, useReducedMotion } from 'motion/react'
import { ArrowUpRight, Check, Archive as ArchiveIcon, CornerDownRight } from 'lucide-react'
import { useCallback, useLayoutEffect, useMemo, useRef, useState, type RefObject } from 'react'
import { createPortal } from 'react-dom'
import { FileReaderLinkProvider, PreviewLinkProvider } from '@/components/session/MessageMarkdown'
import { ComposerCard } from '@/components/session/Composer'
import { OverviewPanel } from '@/components/session/OverviewPanel'
import { Transcript } from '@/components/session/Transcript'
import { deriveSessionView, type SessionView } from '@/components/session/sessionView'
import { IconButton } from '@/components/ui/IconButton'
import { SkeletonRows } from '@/components/ui/Skeleton'
import { toLayoutRect } from '@/lib/dom/zoom'
import { markThreadSeen } from '@/lib/api/feed'
import { sessionMessagesQuery, setSessionArchived } from '@/lib/api/sessions'
import type { FeedItem, Session } from '@/lib/api/types'
import { relativeTime } from '@/lib/format/time'
import { invalidateSessionLists } from '@/lib/query/invalidate'
import { keys } from '@/lib/query/keys'
import { type SendMessageHandler } from '@/lib/sendMessage'
import { COUNTDOWN_SECONDS, useDeferredReply } from './useDeferredReply'

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
  const navigate = useNavigate()
  const reducedMotion = useReducedMotion()
  const openThread = () => navigate({ to: '/sessions/$sessionId', params: { sessionId: item.id } })
  const detail = useQuery({ ...sessionMessagesQuery(item.id), enabled: expanded })
  const view = useMemo(() => (detail.data ? deriveSessionView(detail.data, []) : null), [detail.data])
  const lastTurn = useMemo(() => {
    if (!detail.data || !view) return null
    const sincePrompt = (at: string | undefined) => {
      const time = Date.parse(at ?? '')
      return !Number.isNaN(time) && time >= view.latestUserAt
    }
    return {
      messages: detail.data.messages.filter((message) => sincePrompt(message.created_at)),
      events: view.displayEvents.filter((event) => sincePrompt(event.at)),
    }
  }, [detail.data, view])

  const removeFromFeed = useCallback(() => {
    queryClient.setQueryData<FeedItem[]>(keys.feed, (prev) =>
      (prev ?? []).filter((entry) => entry.id !== item.id),
    )
  }, [queryClient, item.id])

  const { counting, sendDeferred, sendNow, commitNow, cancel } = useDeferredReply(item.id, removeFromFeed)

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
        {expanded && detail.data && view ? (
          <FeedOverview anchorRef={cardRef} session={detail.data.session} view={view} onSend={sendNow} />
        ) : null}
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
              <div className="max-h-[42vh] overflow-y-auto [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
                {lastTurn && detail.data ? (
                  <FileReaderLinkProvider onOpen={openThread}>
                    <PreviewLinkProvider onOpen={openThread}>
                      <Transcript
                        messages={lastTurn.messages}
                        events={lastTurn.events}
                        sessionId={item.id}
                        groupTurns={detail.data.session.runtime === 'acp'}
                        working={detail.data.session.status === 'running'}
                        onArtifactPrompt={sendNow}
                      />
                    </PreviewLinkProvider>
                  </FileReaderLinkProvider>
                ) : (
                  <SkeletonRows count={3} />
                )}
              </div>
              <div className="mt-3">
                <ComposerCard
                  streaming={false}
                  placeholder="Reply…"
                  draftStorageKey={`feed:${item.id}`}
                  attachmentSessionId={item.id}
                  onSend={sendDeferred}
                  onTextChange={cancel}
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
        {counting ? (
          <DoneCountdown onClick={commitNow} />
        ) : (
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
        )}
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

function DoneCountdown({ onClick }: { onClick: () => void }) {
  const circumference = 2 * Math.PI * 11
  return (
    <button
      type="button"
      onClick={onClick}
      aria-label="Send now"
      title="Send now"
      className="relative grid size-7 cursor-pointer place-items-center rounded-full text-ink-2 transition-colors duration-150 hover:bg-ink/10 hover:text-ink"
    >
      <svg viewBox="0 0 28 28" className="absolute inset-0 size-7 -rotate-90" aria-hidden>
        <circle cx="14" cy="14" r="11" fill="none" strokeWidth="2" className="stroke-border" />
        <circle
          cx="14"
          cy="14"
          r="11"
          fill="none"
          strokeWidth="2"
          strokeLinecap="round"
          className="stroke-primary"
          style={{
            strokeDasharray: circumference,
            strokeDashoffset: 0,
            animation: `feed-countdown ${COUNTDOWN_SECONDS}s linear forwards`,
            ['--ring-circumference' as string]: circumference,
          }}
        />
      </svg>
      <Check size={13} />
    </button>
  )
}

function FeedOverview({
  anchorRef,
  session,
  view,
  onSend,
}: {
  anchorRef: RefObject<HTMLElement | null>
  session: Session
  view: SessionView
  onSend: SendMessageHandler
}) {
  const reducedMotion = useReducedMotion()
  const [rect, setRect] = useState<DOMRect | null>(null)

  // The card lives in a scroll container, so the panel is portalled out and
  // pinned with fixed coordinates. The card can move without resizing or
  // scrolling the window (sidebar toggle, cards above expanding), which no
  // ResizeObserver/scroll listener catches — so track its box every frame and
  // only re-render when it actually shifts.
  useLayoutEffect(() => {
    const anchor = anchorRef.current
    if (!anchor) return
    let frame = 0
    let box = ''
    const track = () => {
      const next = anchor.getBoundingClientRect()
      const snapshot = `${next.top}:${next.right}`
      if (snapshot !== box) {
        box = snapshot
        setRect(next)
      }
      frame = requestAnimationFrame(track)
    }
    track()
    return () => cancelAnimationFrame(frame)
  }, [anchorRef])

  if (!rect) return null
  const anchor = toLayoutRect(rect)
  const from = reducedMotion ? 0 : -16
  return createPortal(
    <motion.div
      style={{ position: 'fixed', top: anchor.top, left: anchor.right + 12 }}
      initial={{ opacity: 0, x: from }}
      animate={{ opacity: 1, x: 0 }}
      exit={{ opacity: 0, x: from }}
      transition={{ duration: 0.18, ease: 'easeOut' }}
      className="z-drawer"
    >
      <OverviewPanel
        session={session}
        subagents={view.providerSubagents}
        spawnedThreads={view.spawnedThreads}
        progress={view.panelProgress}
        working={session.status === 'running'}
        onSend={onSend}
      />
    </motion.div>,
    document.body,
  )
}
