import { FileText } from 'lucide-react'
import { memo, useMemo } from 'react'
import type { ACPToolCall, ACPToolContent } from '@/lib/api/types'
import { computeLineDiffHunks } from '@/lib/diff/lineDiff'
import { DiffHunkTable } from './DiffView'

function statusLabel(block: ACPToolContent): string {
  if (!block.old_text) return 'new'
  if (!block.new_text) return 'deleted'
  return ''
}

const EditDiffFile = memo(function EditDiffFile({ block }: { block: ACPToolContent }) {
  const hunks = useMemo(
    () => computeLineDiffHunks(block.old_text ?? '', block.new_text ?? ''),
    [block.old_text, block.new_text],
  )
  const lines = hunks.flatMap((hunk) => hunk.lines)
  const added = lines.filter((line) => line.kind === 'add').length
  const removed = lines.filter((line) => line.kind === 'del').length
  const status = statusLabel(block)

  if (!hunks.length) return null

  return (
    <div className="w-full overflow-hidden rounded-card border border-border">
      <div className="flex items-center gap-2 border-b border-border bg-surface px-2.5 py-1.5">
        <FileText size={12} className="shrink-0 text-ink-3" aria-hidden />
        <span className="min-w-0 flex-1 truncate font-mono text-[12px] text-ink-2" title={block.path}>
          {block.path || 'edit'}
        </span>
        {status ? <span className="shrink-0 text-[11px] text-ink-3">{status}</span> : null}
        <span className="shrink-0 font-mono text-[11px] tabular-nums">
          {added ? <span className="text-ok">+{added}</span> : null}
          {added && removed ? ' ' : null}
          {removed ? <span className="text-danger">−{removed}</span> : null}
          {!added && !removed ? <span className="text-ink-3">±0</span> : null}
        </span>
      </div>
      <DiffHunkTable hunks={hunks} path={block.path ?? ''} flattenTokens />
    </div>
  )
})

export const EditDiffBlock = memo(function EditDiffBlock({ call }: { call: ACPToolCall }) {
  const diffs = (call.content ?? []).filter(isRenderableDiff)
  if (!diffs.length) return null
  return (
    <div className="flex w-full flex-col gap-2">
      {diffs.map((block, index) => (
        <EditDiffFile key={`${block.path ?? 'edit'}-${index}`} block={block} />
      ))}
    </div>
  )
})

function isRenderableDiff(block: ACPToolContent): boolean {
  return block.type === 'diff' && (block.old_text ?? '') !== (block.new_text ?? '')
}

export function hasInlineDiff(call: ACPToolCall): boolean {
  return (call.content ?? []).some(isRenderableDiff)
}
