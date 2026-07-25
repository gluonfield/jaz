import type { SyntaxLine } from '@/lib/code/syntaxHighlight'
import type { DiffHunk } from './parseUnifiedDiff'

export interface DiffSides {
  // The old and new file as this diff sees them: context plus deletions, and
  // context plus additions.
  old: string[]
  new: string[]
  // Where each rendered row's text landed, in row order.
  rows: Array<{ side: 'old' | 'new'; at: number }>
}

// Splits a diff's rows back into the two files they came from. Rendered rows
// interleave deletions and additions, so highlighting them as one document
// describes a file that never existed — an add/del pair can leave a brace or
// quote unbalanced, which both colours tokens from bogus grammar state and can
// send a TextMate grammar into unbounded backtracking on the main thread.
export function diffSides(hunks: DiffHunk[]): DiffSides {
  const sides: DiffSides = { old: [], new: [], rows: [] }
  for (const hunk of hunks) {
    for (const line of hunk.lines) {
      if (line.kind === 'del') {
        sides.rows.push({ side: 'old', at: sides.old.length })
        sides.old.push(line.text)
        continue
      }
      sides.rows.push({ side: 'new', at: sides.new.length })
      sides.new.push(line.text)
      if (line.kind === 'context') sides.old.push(line.text)
    }
  }
  return sides
}

// Reads each row's tokens back out of the side holding its text, restoring the
// one-entry-per-rendered-row shape the diff table renders from. A row whose
// side is still highlighting gets no tokens and renders as plain text.
export function diffTokens(
  sides: DiffSides,
  old: SyntaxLine[] | null,
  next: SyntaxLine[] | null,
): SyntaxLine[] | null {
  if (!old && !next) return null
  return sides.rows.map((row) => (row.side === 'old' ? old : next)?.[row.at] ?? [])
}
