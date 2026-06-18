import { useQuery } from '@tanstack/react-query'
import { Sparkles } from 'lucide-react'
import { AnimatePresence, motion, useReducedMotion } from 'motion/react'
import { useEffect, useLayoutEffect, useRef, useState, type RefObject } from 'react'
import { createPortal } from 'react-dom'
import { skillsQuery } from '@/lib/api/skills'

// Inline mentions persist in message text as markdown-style links whose label
// starts with a sigil: `[$skill-name](/abs/path/SKILL.md)` or
// `[@rel/path](/abs/path)` — the same codec Codex uses for its history. The
// text stays self-describing for the agent (name + full target inline) while
// the UI decodes it back into styled tokens.

export interface Mention {
  sigil: '$' | '@'
  /** visible token text without the sigil */
  name: string
  /** canonical target: absolute path, SKILL.md location, … */
  target: string
}

export type MentionSegment = { text: string; mention?: Mention }

// encodeMention renders a mention back to its wire form. Targets containing
// spaces are wrapped in <> so the link also parses as valid CommonMark when
// assistant prose echoes it.
export function encodeMention(sigil: '$' | '@', name: string, target: string): string {
  const destination = /\s/.test(target) ? `<${target}>` : target
  return `[${sigil}${name}](${destination})`
}

// decodeMentions splits text into plain runs and mention tokens with a single
// linear scan. Only links whose label starts with a sigil are treated as
// mentions; everything else (including ordinary markdown links) passes
// through untouched.
export function decodeMentions(text: string): MentionSegment[] {
  const segments: MentionSegment[] = []
  let plainStart = 0
  let i = 0
  while (i < text.length) {
    const mention = parseMentionAt(text, i)
    if (mention) {
      if (i > plainStart) segments.push({ text: text.slice(plainStart, i) })
      segments.push({
        text: `${mention.sigil}${mention.name}`,
        mention: { sigil: mention.sigil, name: mention.name, target: mention.target },
      })
      i = mention.end
      plainStart = i
    } else {
      i++
    }
  }
  if (plainStart < text.length) segments.push({ text: text.slice(plainStart) })
  return segments
}

function parseMentionAt(
  text: string,
  start: number,
): { sigil: '$' | '@'; name: string; target: string; end: number } | null {
  if (text[start] !== '[') return null
  const sigil = text[start + 1]
  if (sigil !== '$' && sigil !== '@') return null
  const labelEnd = text.indexOf('](', start + 2)
  if (labelEnd === -1) return null
  const name = text.slice(start + 2, labelEnd)
  if (name === '' || name.includes('\n') || name.includes('[')) return null
  const targetEnd = text.indexOf(')', labelEnd + 2)
  if (targetEnd === -1) return null
  let target = text.slice(labelEnd + 2, targetEnd)
  if (target.startsWith('<') && target.endsWith('>')) target = target.slice(1, -1)
  if (target === '' || target.includes('\n')) return null
  return { sigil, name, target, end: targetEnd + 1 }
}

const PILL_CLASS = 'rounded-[4px] bg-primary-soft px-1 py-px text-primary-strong'

export function MentionPill({ mention }: { mention: Mention }) {
  if (mention.sigil === '$') return <SkillMentionPill mention={mention} />
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
      <SkillPopover
        open={open}
        onClose={() => setOpen(false)}
        anchorRef={triggerRef}
        name={mention.name}
        description={description}
      />
    </>
  )
}

const POPOVER_WIDTH = 300
const VIEWPORT_MARGIN = 8
const ANCHOR_GAP = 8

type PopoverPos = { left: number; top?: number; bottom?: number; below: boolean }

function SkillPopover({
  open,
  onClose,
  anchorRef,
  name,
  description,
}: {
  open: boolean
  onClose: () => void
  anchorRef: RefObject<HTMLButtonElement | null>
  name: string
  description?: string
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
          <div className="flex items-start gap-1.5">
            <Sparkles size={13} className="mt-0.5 shrink-0 text-primary" />
            <span className="min-w-0 break-words text-[13px] font-medium text-ink">{name}</span>
          </div>
          <p className="mt-1.5 text-[12.5px] leading-relaxed text-ink-2">
            {description || 'No description available for this skill.'}
          </p>
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
