import { memo } from 'react'
import type { RepoFileChange } from '@/lib/api/types'
import { parseUnifiedDiff, type DiffHunk } from '@/lib/diff/parseUnifiedDiff'

// A file's +/− counts, shared by the panel's changed-file rows and the
// review modal's section headers.
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
  binary,
  truncated,
}: {
  patch: string
  binary?: boolean
  truncated?: boolean
}) {
  if (binary) {
    return <Notice>Binary file — no text diff.</Notice>
  }
  const hunks = parseUnifiedDiff(patch)
  if (!hunks.length) {
    return <Notice>No changes to show.</Notice>
  }
  return (
    <div className="overflow-x-auto font-mono text-[12px] leading-[1.5]">
      <table className="w-full border-separate border-spacing-0">
        <tbody>
          {hunks.map((hunk, index) => (
            <Hunk key={index} hunk={hunk} first={index === 0} />
          ))}
        </tbody>
      </table>
      {truncated ? <Notice>Diff truncated — the full change is larger than this preview.</Notice> : null}
    </div>
  )
})

function Hunk({ hunk, first }: { hunk: DiffHunk; first: boolean }) {
  return (
    <>
      {!first ? (
        <tr aria-hidden>
          <td colSpan={3} className="select-none truncate px-3 py-0.5 text-ink-3">
            ⋯{hunk.context ? <span className="pl-2 opacity-80">{hunk.context}</span> : null}
          </td>
        </tr>
      ) : null}
      {hunk.lines.map((line, index) => {
        const row =
          line.kind === 'add' ? 'bg-ok/10' : line.kind === 'del' ? 'bg-danger/10' : ''
        const marker = line.kind === 'add' ? '+' : line.kind === 'del' ? '−' : ' '
        const markerColor =
          line.kind === 'add' ? 'text-ok' : line.kind === 'del' ? 'text-danger' : 'text-transparent'
        return (
          <tr key={index} className={row}>
            <td className="w-10 min-w-10 select-none border-r border-border/60 pr-2 text-right align-top text-[11px] text-ink-3 tabular-nums">
              {line.oldNo ?? ''}
            </td>
            <td className="w-10 min-w-10 select-none border-r border-border/60 pr-2 text-right align-top text-[11px] text-ink-3 tabular-nums">
              {line.newNo ?? ''}
            </td>
            <td className="whitespace-pre pl-2 pr-3 align-top text-ink-2">
              <span className={`select-none ${markerColor}`}>{marker}</span> {line.text}
            </td>
          </tr>
        )
      })}
    </>
  )
}

function Notice({ children }: { children: string }) {
  return <p className="px-2.5 py-2 text-[12px] italic text-ink-3">{children}</p>
}
