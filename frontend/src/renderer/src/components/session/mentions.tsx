import { useQuery } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { ArrowRight, MessageSquare, Sparkles } from 'lucide-react'
import { AnimatePresence, motion, useReducedMotion } from 'motion/react'
import { useEffect, useLayoutEffect, useRef, useState, type ReactNode, type RefObject } from 'react'
import { createPortal } from 'react-dom'
import { sessionQuery } from '@/lib/api/sessions'
import { skillsQuery } from '@/lib/api/skills'
import { decodeMentions, type Mention } from './mentionCodec'

const PILL_CLASS = 'rounded-[4px] bg-primary-soft px-1 py-px text-primary-strong'
const SESSION_ID_RE = /^\d{8}T\d{6}-[a-f0-9]{8}$/i

export function MentionPill({ mention }: { mention: Mention }) {
  if (mention.sigil === '$') return <SkillMentionPill mention={mention} />
  if (mention.sigil === '@' && SESSION_ID_RE.test(mention.target)) {
    return <ThreadMentionPill mention={mention} />
  }
  return (
    <span title={mention.target} className={PILL_CLASS}>
      {mention.sigil}
      {mention.name}
    </span>
  )
}

function SkillMentionPill({ mention }: { mention: Mention }) {
  const [open, setOpen] = useState(false)
  const { data: skills } = useQuery(skillsQuery())
  const description = skills?.find((s) => s.name === mention.name)?.description
  const triggerRef = useRef<HTMLButtonElement>(null)
  return (
    <>
      <button
        ref={triggerRef}
        type="button"
        title={mention.target}
        onClick={() => setOpen((v) => !v)}
        className={`${PILL_CLASS} cursor-pointer transition-colors hover:bg-primary/20`}
      >
        {mention.sigil}
        {mention.name}
      </button>
      <MentionPopover
        open={open}
        onClose={() => setOpen(false)}
        anchorRef={triggerRef}
      >
        <div className="flex items-start gap-1.5">
          <Sparkles size={13} className="mt-0.5 shrink-0 text-primary" />
          <span className="min-w-0 break-words text-[13px] font-medium text-ink">{mention.name}</span>
        </div>
        <p className="mt-1.5 text-[12.5px] leading-relaxed text-ink-2">
          {description || 'No description available for this skill.'}
        </p>
      </MentionPopover>
    </>
  )
}

function ThreadMentionPill({ mention }: { mention: Mention }) {
  const [open, setOpen] = useState(false)
  const navigate = useNavigate()
  const triggerRef = useRef<HTMLButtonElement>(null)
  const session = useQuery({
    ...sessionQuery(mention.target),
    enabled: open,
  })
  const title = session.data?.title || mention.name
  const slug = session.data?.slug
  const agent = session.data?.runtime_ref?.agent
  return (
    <>
      <button
        ref={triggerRef}
        type="button"
        title={mention.target}
        onClick={() => setOpen((v) => !v)}
        className={`${PILL_CLASS} cursor-pointer transition-colors hover:bg-primary/20`}
      >
        {mention.sigil}
        {title}
      </button>
      <MentionPopover open={open} onClose={() => setOpen(false)} anchorRef={triggerRef}>
        <div className="flex items-start gap-1.5">
          <MessageSquare size={13} className="mt-0.5 shrink-0 text-primary" />
          <span className="min-w-0 break-words text-[13px] font-medium text-ink">{title}</span>
        </div>
        <div className="mt-1.5 space-y-1 text-[12px] leading-relaxed text-ink-2">
          <p className="truncate">{slug || mention.target}</p>
          {agent || session.data?.status ? (
            <p className="truncate">
              {[agent, session.data?.status].filter(Boolean).join(' / ')}
            </p>
          ) : null}
          {session.isError ? <p>Thread details unavailable.</p> : null}
        </div>
        <button
          type="button"
          onClick={() => {
            setOpen(false)
            navigate({ to: '/sessions/$sessionId', params: { sessionId: mention.target } })
          }}
          className="mt-3 flex h-7 w-full items-center gap-2 rounded-full px-2.5 text-left text-[13px] text-ink-2 transition-colors duration-150 hover:bg-surface-2 hover:text-ink"
        >
          <span className="min-w-0 flex-1 truncate">Open thread</span>
          <ArrowRight size={13} className="shrink-0 text-primary" />
        </button>
      </MentionPopover>
    </>
  )
}

const POPOVER_WIDTH = 300
const VIEWPORT_MARGIN = 8
const ANCHOR_GAP = 8

type PopoverPos = { left: number; top?: number; bottom?: number; below: boolean }

function MentionPopover({
  open,
  onClose,
  anchorRef,
  children,
}: {
  open: boolean
  onClose: () => void
  anchorRef: RefObject<HTMLButtonElement | null>
  children: ReactNode
}) {
  const reduce = useReducedMotion()
  const panelRef = useRef<HTMLDivElement>(null)
  const [pos, setPos] = useState<PopoverPos | null>(null)

  useLayoutEffect(() => {
    if (!open) return
    const place = () => {
      const anchor = anchorRef.current
      if (!anchor) return
      const rect = anchor.getBoundingClientRect()
      const left = Math.min(
        Math.max(rect.left + rect.width / 2 - POPOVER_WIDTH / 2, VIEWPORT_MARGIN),
        window.innerWidth - POPOVER_WIDTH - VIEWPORT_MARGIN,
      )
      const below = rect.top < 220
      setPos(
        below
          ? { left, top: rect.bottom + ANCHOR_GAP, below }
          : { left, bottom: window.innerHeight - rect.top + ANCHOR_GAP, below },
      )
    }
    place()
    window.addEventListener('scroll', place, true)
    window.addEventListener('resize', place)
    return () => {
      window.removeEventListener('scroll', place, true)
      window.removeEventListener('resize', place)
    }
  }, [open, anchorRef])

  useEffect(() => {
    if (!open) return
    const onPointerDown = (event: MouseEvent) => {
      const target = event.target as Node
      if (anchorRef.current?.contains(target) || panelRef.current?.contains(target)) return
      onClose()
    }
    const onKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onClose()
    }
    document.addEventListener('mousedown', onPointerDown)
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('mousedown', onPointerDown)
      document.removeEventListener('keydown', onKey)
    }
  }, [open, onClose, anchorRef])

  return createPortal(
    <AnimatePresence>
      {open && pos ? (
        <motion.div
          ref={panelRef}
          data-escape-surface=""
          initial={{ opacity: 0, y: reduce ? 0 : pos.below ? -6 : 6 }}
          animate={{ opacity: 1, y: 0 }}
          exit={{ opacity: 0, y: reduce ? 0 : pos.below ? -6 : 6 }}
          transition={{ duration: 0.14, ease: 'easeOut' }}
          style={{ left: pos.left, top: pos.top, bottom: pos.bottom, width: POPOVER_WIDTH }}
          className="fixed z-tooltip rounded-card bg-surface p-3.5 shadow-raised ring-1 ring-border/70"
        >
          {children}
        </motion.div>
      ) : null}
    </AnimatePresence>,
    document.body,
  )
}

// MentionText renders plain message text (user bubbles) with mentions decoded
// into pills; whitespace handling is inherited from the container.
export function MentionText({ text }: { text: string }) {
  const segments = decodeMentions(text)
  if (segments.length === 1 && !segments[0].mention) return <>{text}</>
  return (
    <>
      {segments.map((segment, index) =>
        segment.mention ? (
          <MentionPill key={index} mention={segment.mention} />
        ) : (
          <span key={index}>{segment.text}</span>
        ),
      )}
    </>
  )
}
