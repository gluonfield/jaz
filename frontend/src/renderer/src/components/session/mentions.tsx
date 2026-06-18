import { useQuery } from '@tanstack/react-query'
import { Sparkles } from 'lucide-react'
import { useState } from 'react'
import { Modal } from '@/components/ui/Modal'
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
  const { data: skills } = useQuery(skillsQuery)
  const description = skills?.find((s) => s.name === mention.name)?.description
  return (
    <>
      <button
        type="button"
        title={mention.target}
        onClick={() => setOpen(true)}
        className={`${PILL_CLASS} cursor-pointer transition-colors hover:bg-primary/20`}
      >
        {mention.sigil}
        {mention.name}
      </button>
      <Modal
        open={open}
        onClose={() => setOpen(false)}
        title={mention.name}
        icon={<Sparkles size={16} />}
        size="sm"
      >
        <p className={`text-[13px] leading-relaxed ${description ? 'text-ink-2' : 'text-ink-3'}`}>
          {description || 'No description available for this skill.'}
        </p>
      </Modal>
    </>
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
