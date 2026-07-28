// The thread's navigation model: one entry per user turn, titled by the prompt
// and previewed by the answer that closed it. Markdown is reduced to the words
// a reader skims rather than rendered, so a preview stays legible at rail size.
// Pure data — no JSX — so the rail can memoize one build per data change.
import type { ChatMessage, SessionEvent } from '@/lib/api/types'
import { messageText } from '@/lib/messageText'

export interface OutlineEntry {
  seq: number
  title: string
  /** the answer's opening, flowed into one block — paragraph breaks would read
   * as blank gaps at preview size */
  preview: string
}

const PREVIEW_PARAGRAPHS = 2
// A preview never reads past its opening, so parsing is bounded rather than
// re-running over whole answers on every streamed delta.
const PREVIEW_SCAN_CHARS = 800
const TITLE_SCAN_CHARS = 400

const fenceLine = /^ {0,3}(?:```|~~~)/
const headingLine = /^ {0,3}#{1,6}\s/
// Table rows and thematic breaks carry no words worth previewing.
const droppedLine = /^ {0,3}(?:\||[-*_]{3,}\s*$)/

function inlineText(line: string): string {
  return line
    .replace(/!\[[^\]]*\]\([^)]*\)/g, '')
    .replace(/\[([^\]]*)\]\([^)]*\)/g, '$1')
    .replace(/<\/?[a-z][^>]*>/gi, '')
    .replace(/^ {0,3}>+ ?/, '')
    .replace(/^\s*(?:[-*+]|\d+[.)])\s+/, '')
    .replace(/^ {0,3}#{1,6}\s+/, '')
    // `_` survives: snake_case identifiers outnumber underscore emphasis here.
    .replace(/\*\*|~~|\*|`+/g, '')
    .replace(/\$\$|\\[[\]()]/g, '')
    .trim()
}

// Markdown collapsed to plain paragraphs: fences, tables, and images drop out;
// links, emphasis, and list markers lose their syntax and keep their text.
export function outlineParagraphs(markdown: string): string[] {
  const paragraphs: string[] = []
  let current: string[] = []
  let fenced = false
  const flush = () => {
    const text = current.join(' ').replace(/\s+/g, ' ').trim()
    if (text) paragraphs.push(text)
    current = []
  }
  for (const raw of markdown.split('\n')) {
    if (fenceLine.test(raw)) {
      fenced = !fenced
      flush()
      continue
    }
    if (fenced || droppedLine.test(raw)) continue
    // A heading stands alone so it can't fuse with the prose under it.
    const heading = headingLine.test(raw)
    if (heading) flush()
    const line = inlineText(raw)
    if (!line) {
      flush()
      continue
    }
    current.push(line)
    if (heading) flush()
  }
  flush()
  return paragraphs
}

interface Answer {
  at: number
  text: string
}

function itemTime(value: string | undefined): number {
  const parsed = Date.parse(value ?? '')
  return Number.isNaN(parsed) ? 0 : parsed
}

// Answer text arrives as ACP event content in agent threads and as assistant
// messages in native ones; the outline reads both the same way.
function answerSources(messages: ChatMessage[], events: SessionEvent[]): Answer[] {
  const fromEvents = events.flatMap((event) => {
    const text = event.type === 'artifact' ? '' : event.content?.trim()
    return text ? [{ at: itemTime(event.at), text }] : []
  })
  const fromMessages = messages.flatMap((message) => {
    const text = message.role === 'assistant' ? messageText(message).trim() : ''
    return text ? [{ at: itemTime(message.created_at), text }] : []
  })
  return [...fromEvents, ...fromMessages].sort((a, b) => a.at - b.at)
}

export function buildOutline(messages: ChatMessage[], events: SessionEvent[]): OutlineEntry[] {
  const prompts = messages.flatMap((message) =>
    message.role === 'user'
      ? [{ seq: message.seq, at: itemTime(message.created_at), text: messageText(message) }]
      : [],
  )
  const sources = answerSources(messages, events)
  let cursor = 0
  return prompts.map((prompt, index) => {
    const next = prompts[index + 1]?.at ?? Infinity
    let answer = ''
    while (cursor < sources.length && sources[cursor].at < next) {
      const source = sources[cursor++]
      if (source.at < prompt.at) continue
      // The turn's closing text is its answer; earlier blocks are its narration.
      answer = source.text
    }
    return {
      seq: prompt.seq,
      title: outlineParagraphs(prompt.text.slice(0, TITLE_SCAN_CHARS))[0] || 'Message',
      preview: outlineParagraphs(answer.slice(0, PREVIEW_SCAN_CHARS))
        .slice(0, PREVIEW_PARAGRAPHS)
        .join(' '),
    }
  })
}
