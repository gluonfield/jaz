import { ChevronRight, LoaderCircle } from 'lucide-react'
import { memo, useState, type ReactNode } from 'react'
import { toolCallCategory } from '@/components/session/toolCallCategory'
import { Collapse } from '@/components/ui/Collapse'
import type { ACPToolCall } from '@/lib/api/types'
import { useInlineDiffs, useInlineShellCommands } from '@/lib/appearance'
import { EditDiffBlock, hasInlineDiff } from './EditDiffBlock'
import { ShellCommandBlock, hasInlineShellCommand } from './ShellCommandBlock'
import { hasToolCallDetail, ToolCallDetail } from './ToolCallContent'
import { normalized } from './TranscriptUtils'

function isRunningToolStatus(status?: string): boolean {
  return ['pending', 'in_progress', 'in-progress', 'running'].includes(normalized(status))
}

// One codex-style phrase for a run of tool calls: "Explored 2 files, ran 1 command".
export function toolRunLabel(calls: ACPToolCall[]): string {
  const phrases: Record<string, (n: number) => string> = {
    web_search: (n) => (n === 1 ? 'searched the web' : `searched the web ${n}×`),
    web_fetch: (n) => `visited ${n} page${n === 1 ? '' : 's'}`,
    edit: (n) => `edited ${n} file${n === 1 ? '' : 's'}`,
    read: (n) => `explored ${n} file${n === 1 ? '' : 's'}`,
    search: (n) => `searched ${n} time${n === 1 ? '' : 's'}`,
    image: (n) => `viewed ${n} image${n === 1 ? '' : 's'}`,
    command: (n) => `ran ${n} command${n === 1 ? '' : 's'}`,
    tool: (n) => `used ${n} tool${n === 1 ? '' : 's'}`,
  }
  const order = ['web_search', 'web_fetch', 'read', 'search', 'command', 'edit', 'image', 'tool']
  const counts = new Map<string, number>()
  let failed = 0
  for (const call of calls) {
    const key = toolCallCategory(call)
    counts.set(key, (counts.get(key) ?? 0) + 1)
    if (normalized(call.status) === 'failed') failed += 1
  }
  const parts = order.flatMap((key) => {
    const count = counts.get(key)
    return count ? [phrases[key](count)] : []
  })
  let label = parts.join(', ')
  label = label.slice(0, 1).toUpperCase() + label.slice(1)
  return failed ? `${label}, ${failed} failed` : label
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
        onClick={() => {
          if (expandable) setOpen((value) => !value)
        }}
        className="-ml-2 inline-flex min-h-10 max-w-full items-center gap-1.5 rounded-control px-2 text-left text-[13px] text-ink-3 transition-[background-color,color,transform] duration-150 motion-reduce:transition-none enabled:hover:bg-surface/65 enabled:hover:text-ink-2 enabled:active:scale-[0.96] disabled:cursor-default"
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
        <div className="relative w-full py-1 before:absolute before:bottom-6 before:left-[11px] before:top-6 before:w-px before:bg-border/75">
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
    <div className="inline-flex min-h-7 max-w-full items-center gap-1.5 self-start rounded-full px-1 font-mono text-[12px] text-ink-3">
      <ChevronRight size={12} className="shrink-0 opacity-30" aria-hidden />
      <span className="min-w-0 truncate">{label}</span>
      {status && !running ? <span className="shrink-0">· {status}</span> : null}
      {running ? <LoaderCircle className="size-3 shrink-0 animate-spin text-running" aria-hidden /> : null}
    </div>
  )
}

export const ToolDisclosure = memo(function ToolDisclosure({
  label,
  calls,
  active = false,
}: {
  label: string
  calls: ACPToolCall[]
  active?: boolean
}) {
  const inlineDiffs = useInlineDiffs()
  const inlineShell = useInlineShellCommands()
  const showDiff = (call: ACPToolCall) => inlineDiffs && hasInlineDiff(call)
  const showShell = (call: ACPToolCall) => inlineShell && hasInlineShellCommand(call)
  if (!calls.some((call) => showDiff(call) || showShell(call))) {
    return <ToolRunDisclosure label={label} calls={calls} active={active} />
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
    <div className="flex w-full flex-col items-start gap-2">
      {rows}
    </div>
  )
})

export function ToolSummary({ calls, active = false }: { calls?: ACPToolCall[]; active?: boolean }) {
  if (!calls?.length) return null
  return <ToolDisclosure label={toolRunLabel(calls)} calls={calls} active={active} />
}
