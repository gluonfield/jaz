import type { DiffLine } from './parseUnifiedDiff'

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
