import { ChevronRight, LoaderCircle } from 'lucide-react'
import { memo, useState, type ReactNode } from 'react'
import { Collapse } from '@/components/ui/Collapse'
import type { ACPToolCall } from '@/lib/api/types'
import { useInlineDiffs, useInlineShellCommands } from '@/lib/appearance'
import { EditDiffBlock, hasInlineDiff } from './EditDiffBlock'
import { ShellCommandBlock, hasInlineShellCommand } from './ShellCommandBlock'
import { ToolCallDetail } from './ToolCallContent'
import { hasToolCallDetail, toolRunLabel } from './toolPresentation'
import { normalized } from './TranscriptUtils'

function isRunningToolStatus(status?: string): boolean {
  return ['pending', 'in_progress', 'in-progress', 'running'].includes(normalized(status))
}

const ToolRunDisclosure = memo(function ToolRunDisclosure({
  label,
  calls,
  active = false,
}: {
  label: string
  calls: ACPToolCall[]
  active?: boolean
}) {
  const [open, setOpen] = useState(false)
  const detailCalls = calls.filter(hasToolCallDetail)
  const expandable = detailCalls.length > 0
  // Old sessions can hold stale non-terminal statuses; only spin while the
  // session is actually working.
  const running = active && calls.some((call) => isRunningToolStatus(call.status))
  return (
    <div className="flex w-full flex-col items-start">
      <button
        type="button"
        disabled={!expandable}
        aria-expanded={expandable ? open : undefined}
        onClick={() => setOpen((value) => !value)}
        className="-ml-1.5 inline-flex min-h-8 max-w-full items-center gap-1.5 rounded-control px-1.5 text-left text-[13px] text-ink-3 transition-colors duration-150 motion-reduce:transition-none enabled:hover:text-ink-2 disabled:cursor-default"
      >
        <span className="min-w-0 truncate">{label}</span>
        {running ? (
          <LoaderCircle className="size-3 animate-spin text-running" aria-hidden />
        ) : null}
        <ChevronRight
          size={13}
          className={`shrink-0 transition-transform duration-150 motion-reduce:transition-none ${!expandable ? 'opacity-30' : open ? 'rotate-90' : ''}`}
          aria-hidden
        />
      </button>
      <Collapse open={open && expandable} className="w-full">
        <div className="relative w-full py-0.5 before:absolute before:bottom-4 before:left-[9px] before:top-4 before:w-px before:bg-border/75">
          {detailCalls.map((call) => (
            <ToolCallDetail key={call.id} call={call} />
          ))}
        </div>
      </Collapse>
    </div>
  )
})

export function ToolStatusLine({
  label,
  status,
  active = false,
}: {
  label: string
  status?: string
  active?: boolean
}) {
  const running = active && isRunningToolStatus(status)
  return (
    <div className="inline-flex min-h-7 max-w-full items-center gap-1.5 self-start rounded-full px-1 text-[12px] text-ink-3">
      <ChevronRight size={12} className="shrink-0 opacity-30" aria-hidden />
      <span className="min-w-0 truncate">{label}</span>
      {status && !running ? <span className="shrink-0">· {status}</span> : null}
      {running ? <LoaderCircle className="size-3 shrink-0 animate-spin text-running" aria-hidden /> : null}
    </div>
  )
}

export const ToolDisclosure = memo(function ToolDisclosure({
  calls,
  active = false,
}: {
  calls: ACPToolCall[]
  active?: boolean
}) {
  const inlineDiffs = useInlineDiffs()
  const inlineShell = useInlineShellCommands()
  const showDiff = (call: ACPToolCall) => inlineDiffs && hasInlineDiff(call)
  const showShell = (call: ACPToolCall) => inlineShell && hasInlineShellCommand(call)
  if (!calls.some((call) => showDiff(call) || showShell(call))) {
    return <ToolRunDisclosure label={toolRunLabel(calls)} calls={calls} active={active} />
  }

  const rows: ReactNode[] = []
  let run: ACPToolCall[] = []
  const flushRun = () => {
    if (!run.length) return
    rows.push(
      <ToolRunDisclosure
        key={`run-${rows.length}-${run[0].id}`}
        label={toolRunLabel(run)}
        calls={run}
        active={active}
      />,
    )
    run = []
  }
  for (const call of calls) {
    if (showDiff(call)) {
      flushRun()
      rows.push(<EditDiffBlock key={call.id} call={call} />)
      continue
    }
    if (showShell(call)) {
      flushRun()
      rows.push(<ShellCommandBlock key={call.id} call={call} active={active} />)
      continue
    }
    run.push(call)
  }
  flushRun()

  return (
    <div className="flex w-full flex-col items-start gap-1">
      {rows}
    </div>
  )
})

export function ToolSummary({ calls, active = false }: { calls?: ACPToolCall[]; active?: boolean }) {
  if (!calls?.length) return null
  return <ToolDisclosure calls={calls} active={active} />
}
