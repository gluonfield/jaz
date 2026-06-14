import { ChevronDown } from 'lucide-react'
import { memo, useMemo } from 'react'
import {
  HighlightedCodeLine,
  useSyntaxHighlightedLines,
  type HighlightedCodeLines,
} from '@/components/session/HighlightedCode'
import type { RepoFileChange } from '@/lib/api/types'
import { parseUnifiedDiff, type DiffHunk } from '@/lib/diff/parseUnifiedDiff'

// A file's +/− counts, shared by changed-file rows and diff section headers.
export function FileCounts({ file }: { file: RepoFileChange }) {
  if (file.binary) return <span className="shrink-0 text-[11px] text-ink-3">binary</span>
  return (
    <span className="shrink-0 font-mono text-[11px] tabular-nums">
      {file.added ? <span className="text-ok">+{file.added}</span> : null}
      {file.added && file.deleted ? ' ' : null}
      {file.deleted ? <span className="text-danger">−{file.deleted}</span> : null}
      {!file.added && !file.deleted ? <span className="text-ink-3">±0</span> : null}
    </span>
  )
}

// Renders one file's unified diff: dual line-number gutters, green/red rows,
// and an ellipsis row where unmodified lines are collapsed (git's own hunk
// boundaries — no client-side folding needed).
export const DiffView = memo(function DiffView({
  patch,
  path,
  binary,
  truncated,
}: {
  patch: string
  path?: string
  binary?: boolean
  truncated?: boolean
}) {
  const hunks = useMemo(() => parseUnifiedDiff(patch), [patch])
  const lines = useMemo(() => hunks.flatMap((hunk) => hunk.lines.map((line) => line.text)), [hunks])
  const highlighted = useSyntaxHighlightedLines(binary ? '' : (path ?? ''), lines)
  if (binary) {
    return <Notice>Binary file — no text diff.</Notice>
  }
  if (!hunks.length) {
    return <Notice>No changes to show.</Notice>
  }
  let offset = 0
  return (
    <div className="overflow-x-auto bg-bg/45 font-mono text-[12px] leading-[1.55] select-text">
      <table className="w-full min-w-max border-separate border-spacing-0">
        <tbody>
          {hunks.map((hunk, index) => {
            const start = offset
            offset += hunk.lines.length
            return (
              <Hunk
                key={index}
                hunk={hunk}
                previous={hunks[index - 1]}
                highlighted={highlighted?.slice(start, offset)}
              />
            )
          })}
        </tbody>
      </table>
      {truncated ? <Notice>Diff truncated — the full change is larger than this preview.</Notice> : null}
    </div>
  )
})

function Hunk({
  hunk,
  previous,
  highlighted,
}: {
  hunk: DiffHunk
  previous?: DiffHunk
  highlighted?: HighlightedCodeLines | null
}) {
  const collapsed = collapsedLineCount(previous, hunk)
  return (
    <>
      {previous ? (
        <tr>
          <td
            colSpan={4}
            className="border-y border-border/50 bg-bg/65 px-3 py-3 text-[12px] text-ink-3 select-none"
          >
            <span className="inline-flex items-center gap-3">
              <ChevronDown size={13} className="text-ink-3/70" aria-hidden />
              <span>
                {collapsed > 0 ? `${collapsed} unmodified ${collapsed === 1 ? 'line' : 'lines'}` : 'Unmodified lines'}
                {hunk.context ? <span className="pl-3 opacity-75">{hunk.context}</span> : null}
              </span>
            </span>
          </td>
        </tr>
      ) : null}
      {hunk.lines.map((line, index) => {
        const row =
          line.kind === 'add'
            ? 'bg-ok/18'
            : line.kind === 'del'
              ? 'bg-danger/18'
              : 'bg-transparent'
        const gutter =
          line.kind === 'add'
            ? 'bg-ok/10 text-ok'
            : line.kind === 'del'
              ? 'bg-danger/10 text-danger'
              : 'text-ink-3'
        const marker = line.kind === 'add' ? '+' : line.kind === 'del' ? '−' : ' '
        const markerColor =
          line.kind === 'add' ? 'text-ok' : line.kind === 'del' ? 'text-danger' : 'text-transparent'
        return (
          <tr key={index} className={row}>
            <td className={`w-11 min-w-11 pr-2 text-right align-top text-[11px] tabular-nums select-none ${gutter}`}>
              {line.oldNo ?? ''}
            </td>
            <td className={`w-11 min-w-11 pr-2 text-right align-top text-[11px] tabular-nums select-none ${gutter}`}>
              {line.newNo ?? ''}
            </td>
            <td className={`w-5 min-w-5 text-center align-top select-none ${markerColor}`}>{marker}</td>
            <td className="whitespace-pre pr-5 align-top text-ink-2 select-text">
              <HighlightedCodeLine text={line.text} tokens={highlighted?.[index]} />
            </td>
          </tr>
        )
      })}
    </>
  )
}

function collapsedLineCount(previous: DiffHunk | undefined, hunk: DiffHunk): number {
  if (!previous) return 0
  const oldGap = hunk.oldStart - (previous.oldStart + previous.oldLines)
  const newGap = hunk.newStart - (previous.newStart + previous.newLines)
  return Math.max(0, oldGap, newGap)
}

function Notice({ children }: { children: string }) {
  return <p className="px-3 py-3 text-[12px] italic text-ink-3">{children}</p>
}
