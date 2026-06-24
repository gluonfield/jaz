import type { DiffHunk, DiffLine } from './parseUnifiedDiff'

// Splits text into lines, dropping the single empty element a trailing newline
// produces so a final "\n" doesn't render as a phantom blank line.
function splitLines(text: string): string[] {
  const lines = text.split('\n')
  if (lines.length > 1 && lines[lines.length - 1] === '') lines.pop()
  return lines
}

// Computes a line-level diff of two texts via longest-common-subsequence and
// returns rows tagged context/add/del with old/new line numbers — the same
// DiffLine shape parseUnifiedDiff yields, so the renderer is shared. Inputs come
// from ACP `diff` tool-content blocks, which the backend clamps to a few KB, so
// the O(n·m) table stays small.
export function computeLineDiff(oldText: string, newText: string): DiffLine[] {
  const a = oldText ? splitLines(oldText) : []
  const b = newText ? splitLines(newText) : []
  const n = a.length
  const m = b.length

  const dp: number[][] = Array.from({ length: n + 1 }, () => new Array<number>(m + 1).fill(0))
  for (let i = n - 1; i >= 0; i--) {
    for (let j = m - 1; j >= 0; j--) {
      dp[i][j] = a[i] === b[j] ? dp[i + 1][j + 1] + 1 : Math.max(dp[i + 1][j], dp[i][j + 1])
    }
  }

  const out: DiffLine[] = []
  let i = 0
  let j = 0
  let oldNo = 1
  let newNo = 1
  while (i < n && j < m) {
    if (a[i] === b[j]) {
      out.push({ kind: 'context', oldNo: oldNo++, newNo: newNo++, text: a[i] })
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
