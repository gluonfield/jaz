export interface Mention {
  sigil: '$' | '@'
  /** visible token text without the sigil */
  name: string
  /** canonical target: absolute path, SKILL.md location, thread id, ... */
  target: string
}

export type MentionSegment = { text: string; mention?: Mention }

export function mentionLabelText(value: string): string {
  return value.replace(/\s+/g, ' ').replaceAll('[', '').replaceAll(']', '').trim()
}

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
