import { FileText } from 'lucide-react'
import { memo, useMemo } from 'react'
import type { ACPToolCall, ACPToolContent } from '@/lib/api/types'
import { computeLineDiff } from '@/lib/diff/lineDiff'
import type { DiffLine } from '@/lib/diff/parseUnifiedDiff'
import { HighlightedCodeLine, useSyntaxHighlightedLines } from './HighlightedCode'

const CONTEXT = 3

type Row = { kind: 'line'; line: DiffLine; index: number } | { kind: 'gap'; count: number }

// Folds long runs of unchanged lines down to a few lines of context on each side
// of a change, with a marker row standing in for the rest. Edit blocks are
// usually small, so most runs pass through untouched.
function collapseContext(lines: DiffLine[]): Row[] {
  const rows: Row[] = []
  let i = 0
  while (i < lines.length) {
    if (lines[i].kind !== 'context') {
      rows.push({ kind: 'line', line: lines[i], index: i })
      i++
      continue
    }
    let j = i
    while (j < lines.length && lines[j].kind === 'context') j++
    const runStart = i
    const runLen = j - i
    const atStart = i === 0
    const atEnd = j === lines.length
    const head = atStart ? 0 : CONTEXT
    const tail = atEnd ? 0 : CONTEXT
    if (runLen <= head + tail + 1) {
      for (let k = runStart; k < j; k++) rows.push({ kind: 'line', line: lines[k], index: k })
    } else {
      for (let k = runStart; k < runStart + head; k++) rows.push({ kind: 'line', line: lines[k], index: k })
      rows.push({ kind: 'gap', count: runLen - head - tail })
      for (let k = j - tail; k < j; k++) rows.push({ kind: 'line', line: lines[k], index: k })
    }
    i = j
  }
  return rows
}

function statusLabel(block: ACPToolContent): string {
  if (!block.old_text) return 'new'
  if (!block.new_text) return 'deleted'
  return ''
}

const EditDiffFile = memo(function EditDiffFile({ block }: { block: ACPToolContent }) {
  const lines = useMemo(
    () => computeLineDiff(block.old_text ?? '', block.new_text ?? ''),
    [block.old_text, block.new_text],
  )
  const codeLines = useMemo(() => lines.map((line) => line.text), [lines])
  const highlighted = useSyntaxHighlightedLines(block.path ?? '', codeLines)
  const rows = useMemo(() => collapseContext(lines), [lines])
  const added = lines.reduce((n, line) => n + (line.kind === 'add' ? 1 : 0), 0)
  const removed = lines.reduce((n, line) => n + (line.kind === 'del' ? 1 : 0), 0)
  const status = statusLabel(block)
  // Drop a line-number column that never carries a number — a new file has no old
  // numbers, a deletion no new ones — so a pure add/del doesn't waste a gutter's
  // width as dead space on the left.
  const hasOld = lines.some((line) => line.oldNo != null)
  const hasNew = lines.some((line) => line.newNo != null)
  const colSpan = (hasOld ? 1 : 0) + (hasNew ? 1 : 0) + 2
  const gutterBase = 'w-8 min-w-8 pr-2 text-right align-top text-[11px] tabular-nums select-none'

  if (!lines.length) return null

  return (
    <div className="w-full overflow-hidden rounded-card border border-border">
      <div className="flex items-center gap-2 border-b border-border bg-surface px-2.5 py-1.5">
        <FileText size={12} className="shrink-0 text-ink-3" aria-hidden />
        <span className="min-w-0 flex-1 truncate font-mono text-[12px] text-ink-2" title={block.path}>
          {block.path || 'edit'}
        </span>
        {status ? <span className="shrink-0 text-[11px] text-ink-3">{status}</span> : null}
        <span className="shrink-0 font-mono text-[11px] tabular-nums">
          {added ? <span className="text-[var(--diff-add-fg)]">+{added}</span> : null}
          {added && removed ? ' ' : null}
          {removed ? <span className="text-[var(--diff-del-fg)]">−{removed}</span> : null}
          {!added && !removed ? <span className="text-ink-3">±0</span> : null}
        </span>
      </div>
      <div className="overflow-x-auto bg-bg py-2.5 font-mono text-[12px] leading-[1.55] select-text">
        <table className="w-full min-w-max border-separate border-spacing-0">
          <tbody>
            {rows.map((row, rowIndex) => {
              if (row.kind === 'gap') {
                return (
                  <tr key={`gap-${rowIndex}`}>
                    <td
                      colSpan={colSpan}
                      className="border-y border-border/50 bg-bg/65 px-3 py-1.5 text-[11px] text-ink-3 select-none"
                    >
                      {row.count} unmodified {row.count === 1 ? 'line' : 'lines'}
                    </td>
                  </tr>
                )
              }
              const { line, index } = row
              const rowBg =
                line.kind === 'add'
                  ? 'bg-[var(--diff-add-bg)]'
                  : line.kind === 'del'
                    ? 'bg-[var(--diff-del-bg)]'
                    : 'bg-transparent'
              const gutter =
                line.kind === 'add'
                  ? 'bg-[var(--diff-add-gutter)] text-[var(--diff-add-fg)]'
                  : line.kind === 'del'
                    ? 'bg-[var(--diff-del-gutter)] text-[var(--diff-del-fg)]'
                    : 'text-ink-3'
              const marker = line.kind === 'add' ? '+' : line.kind === 'del' ? '−' : ' '
              const markerColor =
                line.kind === 'add'
                  ? 'text-[var(--diff-add-fg)]'
                  : line.kind === 'del'
                    ? 'text-[var(--diff-del-fg)]'
                    : 'text-transparent'
              return (
                <tr key={index} className={rowBg}>
                  {hasOld ? (
                    <td className={`${gutterBase} pl-3 ${gutter}`}>{line.oldNo ?? ''}</td>
                  ) : null}
                  {hasNew ? (
                    <td className={`${gutterBase} ${hasOld ? '' : 'pl-3'} ${gutter}`}>{line.newNo ?? ''}</td>
                  ) : null}
                  <td className={`w-5 min-w-5 text-center align-top select-none ${markerColor}`}>{marker}</td>
                  <td className="whitespace-pre pr-5 pl-1 align-top text-ink select-text">
                    <HighlightedCodeLine text={line.text} tokens={highlighted?.[index]} flatten />
                  </td>
                </tr>
              )
            })}
          </tbody>
        </table>
      </div>
    </div>
  )
})

// Renders the red/green diffs carried by an agent edit tool call inline in the
// transcript, one bordered panel per file. Used when the inline-diffs appearance
// setting is on; returns null for calls without diff content.
export const EditDiffBlock = memo(function EditDiffBlock({ call }: { call: ACPToolCall }) {
  const diffs = (call.content ?? []).filter((block) => block.type === 'diff')
  if (!diffs.length) return null
  return (
    <div className="flex w-full flex-col gap-2">
      {diffs.map((block, index) => (
        <EditDiffFile key={`${block.path ?? 'edit'}-${index}`} block={block} />
      ))}
    </div>
  )
})

// True when a call is an agent edit that carries renderable diff content.
export function hasInlineDiff(call: ACPToolCall): boolean {
  return (call.content ?? []).some((block) => block.type === 'diff' && (block.old_text || block.new_text))
}
