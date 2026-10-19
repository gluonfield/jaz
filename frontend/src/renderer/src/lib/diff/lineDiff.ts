import type { DiffHunk, DiffLine } from './parseUnifiedDiff'

// Splits text into lines, dropping the single empty element a trailing newline
// produces so a final "\n" doesn't render as a phantom blank line.
function splitLines(text: string): string[] {
  if (!text) {
    return []
  }
  const lines = text.split('\n')
  if (lines.length > 1 && lines[lines.length - 1] === '') lines.pop()
  return lines
}

// Computes a line-level diff of two texts via longest-common-subsequence and
// returns rows tagged context/add/del with old/new line numbers — the same
// DiffLine shape parseUnifiedDiff yields, so the renderer is shared. Inputs come
// from ACP `diff` tool-content blocks, which the backend clamps to a few KB, so
// the O(n·m) table stays small. Lines are kept as context when same says so,
// carrying the new text.
export function computeLineDiff(oldText: string, newText: string, same = (oldLine: string, newLine: string) => oldLine === newLine): DiffLine[] {
  const a = splitLines(oldText)
  const b = splitLines(newText)
  const n = a.length
  const m = b.length

  const dp: number[][] = Array.from({ length: n + 1 }, () => new Array<number>(m + 1).fill(0))
  for (let i = n - 1; i >= 0; i--) {
    for (let j = m - 1; j >= 0; j--) {
      dp[i][j] = same(a[i], b[j]) ? dp[i + 1][j + 1] + 1 : Math.max(dp[i + 1][j], dp[i][j + 1])
    }
  }

  const out: DiffLine[] = []
  let i = 0
  let j = 0
  let oldNo = 1
  let newNo = 1
  while (i < n && j < m) {
    if (same(a[i], b[j])) {
      out.push({ kind: 'context', oldNo: oldNo++, newNo: newNo++, text: b[j] })
      i++
      j++
    } else if (dp[i + 1][j] >= dp[i][j + 1]) {
      out.push({ kind: 'del', oldNo: oldNo++, text: a[i] })
      i++
    } else {
      out.push({ kind: 'add', newNo: newNo++, text: b[j] })
      j++
    }
  }
  while (i < n) out.push({ kind: 'del', oldNo: oldNo++, text: a[i++] })
  while (j < m) out.push({ kind: 'add', newNo: newNo++, text: b[j++] })
  return out
}

function numberedStart(lines: DiffLine[], key: 'oldNo' | 'newNo'): number {
  return lines.find((line) => line[key] != null)?.[key] ?? 0
}

function numberedCount(lines: DiffLine[], key: 'oldNo' | 'newNo'): number {
  return lines.reduce((count, line) => count + (line[key] == null ? 0 : 1), 0)
}

export function computeLineDiffHunks(oldText: string, newText: string, context = 3): DiffHunk[] {
  const lines = computeLineDiff(oldText, newText)
  const changed = lines.flatMap((line, index) => (line.kind === 'context' ? [] : [index]))
  if (!changed.length) return []

  const ranges: Array<{ start: number; end: number }> = []
  for (const index of changed) {
    const start = Math.max(0, index - context)
    const end = Math.min(lines.length - 1, index + context)
    const previous = ranges.at(-1)
    if (previous && start <= previous.end + 1) {
      previous.end = Math.max(previous.end, end)
    } else {
      ranges.push({ start, end })
    }
  }

  return ranges.map(({ start, end }) => {
    const hunkLines = lines.slice(start, end + 1)
    return {
      oldStart: numberedStart(hunkLines, 'oldNo'),
      oldLines: numberedCount(hunkLines, 'oldNo'),
      newStart: numberedStart(hunkLines, 'newNo'),
      newLines: numberedCount(hunkLines, 'newNo'),
      context: '',
      lines: hunkLines,
    }
  })
}

// carryEdits applies the line edits that turn base into edited to source, a
// text base renders differently: render gives a source line as base would
// write it. Source keeps its own text everywhere the edits leave alone; where
// an edit and a difference between base and source come within margin of
// each other (see spanStart), the edit wins. A trailing newline on source
// survives.
export function carryEdits(base: string, edited: string, source: string, render: (line: string) => string, margin: number): string {
  const rendered = new Map(splitLines(source).map((line) => [line, render(line)]))
  const fromSource = computeLineDiff(base, source, (line, text) => line === text || line === rendered.get(text))
  const lines = splitLines(base)
  for (const line of fromSource) {
    if (line.kind === 'context') {
      lines[line.oldNo! - 1] = line.text
    }
  }
  const all = [...hunks(fromSource, false), ...hunks(computeLineDiff(base, edited), true)].sort((a, b) => spanStart(a) - spanStart(b) || spanEnd(a) - spanEnd(b))
  const out: string[] = []
  let at = 0
  for (let first = 0; first < all.length; ) {
    let next = first + 1
    let reach = spanEnd(all[first])
    while (next < all.length && spanStart(all[next]) <= reach + margin) {
      reach = Math.max(reach, spanEnd(all[next]))
      next++
    }
    const group = all.slice(first, next)
    const from = Math.min(...group.map((hunk) => hunk.from))
    const to = Math.max(...group.map((hunk) => hunk.to))
    out.push(...lines.slice(at, from))
    at = from
    for (const hunk of group.some((hunk) => hunk.edit) ? group.filter((hunk) => hunk.edit) : group) {
      out.push(...lines.slice(at, hunk.from), ...hunk.lines)
      at = hunk.to
    }
    out.push(...lines.slice(at, to))
    at = to
    first = next
  }
  out.push(...lines.slice(at))
  return out.join('\n') + (source.endsWith('\n') ? '\n' : '')
}

// A Hunk replaces base lines [from, to) with lines; edit marks the edits'
// hunks apart from source's.
interface Hunk {
  from: number
  to: number
  lines: string[]
  edit: boolean
}

function hunks(diff: DiffLine[], edit: boolean): Hunk[] {
  const out: Hunk[] = []
  let at = 0
  let open: Hunk | undefined
  for (const line of diff) {
    if (line.kind === 'context') {
      at++
      open = undefined
      continue
    }
    if (!open) {
      open = { from: at, to: at, lines: [], edit }
      out.push(open)
    }
    if (line.kind === 'del') {
      at++
      open.to = at
    } else {
      open.lines.push(line.text)
    }
  }
  return out
}

// Spans place a hunk on an axis where base line i sits at 2i+1 and the gap
// before it at 2i. With no margin, two hunks meet when they replace a common
// line, or insert at the same gap or inside the lines the other replaces; a
// margin of 2 also joins hunks on neighbouring lines.
function spanStart(hunk: Hunk): number {
  return hunk.from === hunk.to ? 2 * hunk.from : 2 * hunk.from + 1
}

function spanEnd(hunk: Hunk): number {
  return hunk.from === hunk.to ? 2 * hunk.from : 2 * hunk.to - 1
}
