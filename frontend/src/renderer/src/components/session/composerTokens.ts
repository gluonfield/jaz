// Pure logic for the composer's inline $skill / @path tokens. Tokens live as
// plain display text inside the textarea value (e.g. `$jazmem`,
// `@frontend/src`); this module finds the active trigger under the caret,
// segments the value into plain/token runs (powering both the blue overlay and
// submit-time expansion, so what's highlighted is exactly what expands), and
// handles atomic deletion. The store is keyed by display string: every
// occurrence of a display behaves identically, including hand-typed copies.

export interface InlineToken {
  trigger: '$' | '@'
  /** the literal text in the textarea, e.g. '@frontend/src' */
  display: string
  /** what the display becomes in the sent message, e.g. an absolute path */
  expansion: string
}

export interface ActiveTrigger {
  trigger: '$' | '@' | '/'
  /** index of the trigger character in the value */
  start: number
  /** text between the trigger character and the caret */
  query: string
}

export interface Segment {
  text: string
  token?: InlineToken
}

// findActiveTrigger reports the $ or @ run the caret currently sits in, or
// null. A trigger only counts at the start of a word (so `email@host` never
// opens the menu) and its query cannot span whitespace (so typing a space
// closes the menu).
export function findActiveTrigger(value: string, caret: number): ActiveTrigger | null {
  if (caret < 1 || caret > value.length) return null
  if (value.startsWith('/') && !/\s/.test(value.slice(1, caret))) {
    return { trigger: '/', start: 0, query: value.slice(1, caret) }
  }
  for (let i = caret - 1; i >= 0; i--) {
    const ch = value[i]
    if (/\s/.test(ch)) return null
    if (ch === '$' || ch === '@') {
      if (i > 0 && !/\s/.test(value[i - 1])) return null
      return { trigger: ch, start: i, query: value.slice(i + 1, caret) }
    }
  }
  return null
}

// A display only counts as a token occurrence at word boundaries: preceded by
// whitespace/start (so `mail@src` is not a token) and not followed by a word
// or path character (so hand-typed `@srcery` or `@src/main.go` never
// half-highlights a tagged `@src`). Trailing punctuation is fine — "see
// @src, please" keeps the token.
function isTokenAt(value: string, start: number, display: string): boolean {
  if (!value.startsWith(display, start)) return false
  if (start > 0 && !/\s/.test(value[start - 1])) return false
  const after = value[start + display.length]
  return after === undefined || !/[A-Za-z0-9_./\\:-]/.test(after)
}

// segmentValue splits value into plain and token segments with a single
// left-to-right scan, trying token displays longest-first at each position so
// a display that prefixes another can never shadow it.
export function segmentValue(value: string, tokens: Map<string, InlineToken>): Segment[] {
  if (tokens.size === 0) return value ? [{ text: value }] : []
  const displays = [...tokens.keys()].sort((a, b) => b.length - a.length)
  const segments: Segment[] = []
  let plainStart = 0
  let i = 0
  while (i < value.length) {
    const display = displays.find((d) => isTokenAt(value, i, d))
    if (display) {
      if (i > plainStart) segments.push({ text: value.slice(plainStart, i) })
      segments.push({ text: display, token: tokens.get(display) })
      i += display.length
      plainStart = i
    } else {
      i++
    }
  }
  if (plainStart < value.length) segments.push({ text: value.slice(plainStart) })
  return segments
}

// pruneTokens drops tokens whose display no longer occurs in the value (the
// user edited inside one — it degrades to plain text). Returns the same map
// reference when nothing changed so React state stays referentially stable.
export function pruneTokens(
  tokens: Map<string, InlineToken>,
  value: string,
): Map<string, InlineToken> {
  let pruned: Map<string, InlineToken> | null = null
  for (const display of tokens.keys()) {
    if (!value.includes(display)) {
      pruned ??= new Map(tokens)
      pruned.delete(display)
    }
  }
  return pruned ?? tokens
}

// expandTokens produces the message text to send: token segments become their
// expansion, plain segments pass through.
export function expandTokens(value: string, tokens: Map<string, InlineToken>): string {
  return segmentValue(value, tokens)
    .map((segment) => (segment.token ? segment.token.expansion : segment.text))
    .join('')
}

// tokenEndingAt returns the token whose display ends exactly at caret, with
// its start offset — the atomic-backspace target. Segment-based, so text that
// merely contains a display mid-word can't false-positive.
export function tokenEndingAt(
  value: string,
  caret: number,
  tokens: Map<string, InlineToken>,
): { token: InlineToken; start: number } | null {
  let offset = 0
  for (const segment of segmentValue(value, tokens)) {
    offset += segment.text.length
    if (segment.token && offset === caret) return { token: segment.token, start: caret - segment.text.length }
    if (offset >= caret) return null
  }
  return null
}

let graphemeSegmenter: Intl.Segmenter | null | undefined

function prevGraphemeStart(value: string, caret: number): number {
  if (caret <= 0) return 0
  if (graphemeSegmenter === undefined) {
    graphemeSegmenter =
      typeof Intl !== 'undefined' && 'Segmenter' in Intl
        ? new Intl.Segmenter(undefined, { granularity: 'grapheme' })
        : null
  }
  if (graphemeSegmenter) {
    let last = 0
    for (const { index } of graphemeSegmenter.segment(value.slice(0, caret))) {
      last = index
    }
    return last
  }
  const prev = value.charCodeAt(caret - 1)
  if (caret >= 2 && prev >= 0xdc00 && prev <= 0xdfff) {
    const lead = value.charCodeAt(caret - 2)
    if (lead >= 0xd800 && lead <= 0xdbff) return caret - 2
  }
  return caret - 1
}

function prevWordStart(value: string, caret: number): number {
  let pos = caret
  if (pos > 0 && value[pos - 1] === '\n') return pos - 1
  while (pos > 0 && (value[pos - 1] === ' ' || value[pos - 1] === '\t')) pos--
  if (pos === 0 || value[pos - 1] === '\n') return pos
  const wordChar = /[\p{L}\p{N}_]/u
  const matchWord = wordChar.test(value[pos - 1])
  while (pos > 0 && value[pos - 1] !== '\n' && value[pos - 1] !== ' ' && value[pos - 1] !== '\t') {
    if (wordChar.test(value[pos - 1]) !== matchWord) break
    pos--
  }
  return pos
}

export function backwardDeleteRange(
  value: string,
  start: number,
  end: number,
  modifiers: { meta?: boolean; alt?: boolean; ctrl?: boolean } = {},
): { from: number; to: number } | null {
  if (start !== end) {
    return { from: Math.min(start, end), to: Math.max(start, end) }
  }
  if (start <= 0 || start > value.length) return null
  if (modifiers.meta) {
    const lineStart = value.lastIndexOf('\n', start - 1) + 1
    return { from: lineStart < start ? lineStart : start - 1, to: start }
  }
  if (modifiers.alt || modifiers.ctrl) {
    return { from: prevWordStart(value, start), to: start }
  }
  return { from: prevGraphemeStart(value, start), to: start }
}
