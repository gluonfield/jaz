import { ChevronRight, LoaderCircle } from 'lucide-react'
import { memo, useState, type ReactNode } from 'react'
import { toolCallCategory } from '@/components/session/toolCallCategory'
import type { ACPToolCall } from '@/lib/api/types'
import { useInlineDiffs, useInlineShellCommands } from '@/lib/appearance'
import { EditDiffBlock, hasInlineDiff } from './EditDiffBlock'
import { ShellCommandBlock, hasInlineShellCommand } from './ShellCommandBlock'
import { hasToolCallDetail, ToolCallDetail } from './ToolCallContent'
import { normalized } from './TranscriptUtils'

interface ToolGroup {
  key: string
  label: string
  calls: ACPToolCall[]
}

function toolGroupBaseLabel(key: string, count: number): string {
  const plural = count === 1 ? '' : 's'
  switch (key) {
    case 'web_search':
      return count === 1 ? 'Searched the web' : `Searched the web ${count}×`
    case 'web_fetch':
      return `Visited ${count} page${plural}`
    case 'edit':
      return `Edited ${count} file${plural}`
    case 'read':
      return `Read ${count} file${plural}`
    case 'search':
      return `Searched ${count} time${plural}`
    case 'image':
      return `Viewed ${count} image${plural}`
    case 'command':
      return `Ran ${count} command${plural}`
    default:
      return `Used ${count} tool${plural}`
  }
}

function groupToolCalls(calls: ACPToolCall[]): ToolGroup[] {
  const order = ['web_search', 'web_fetch', 'edit', 'read', 'search', 'image', 'command', 'tool']
  const byKey = new Map<string, ACPToolCall[]>()
  for (const call of calls) {
    const key = toolCallCategory(call)
    byKey.set(key, [...(byKey.get(key) ?? []), call])
  }
  return order.flatMap((key) => {
    const groupCalls = byKey.get(key) ?? []
    if (!groupCalls.length) return []
    const failed = groupCalls.filter((call) => normalized(call.status) === 'failed').length
    const suffix = failed ? `, ${failed} failed` : ''
    return [{ key, label: `${toolGroupBaseLabel(key, groupCalls.length)}${suffix}`, calls: groupCalls }]
  })
}

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

// The collapsible run summary: a chevron + codex-style phrase that expands to
// each call's detail. This is the stock rendering for every tool run.
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
    <div className="flex flex-col items-start gap-1">
      <button
        type="button"
        disabled={!expandable}
        aria-expanded={expandable ? open : undefined}
        onClick={() => {
          if (expandable) setOpen((value) => !value)
        }}
        className="inline-flex min-h-7 items-center gap-1.5 rounded-full px-1 text-left font-mono text-[12px] text-ink-3 transition-colors enabled:hover:text-ink disabled:cursor-default"
      >
        <ChevronRight
          size={12}
          className={`shrink-0 transition-transform ${!expandable ? 'opacity-30' : open ? 'rotate-90' : ''}`}
          aria-hidden
        />
        {label}
        {running ? (
          <LoaderCircle className="size-3 animate-spin text-running" aria-hidden />
        ) : null}
      </button>
      {open && expandable ? (
        <div className="flex w-full max-w-full flex-col gap-1.5 pl-4">
          {detailCalls.map((call) =>
            hasInlineDiff(call) ? (
              <EditDiffBlock key={call.id} call={call} />
            ) : hasInlineShellCommand(call) ? (
              <ShellCommandBlock key={call.id} call={call} active={active} />
            ) : (
              <ToolCallDetail key={call.id} call={call} />
            ),
          )}
        </div>
      ) : null}
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
  return (
    <div className="flex flex-col items-start gap-1.5">
      {groupToolCalls(calls).map((group) => (
        <ToolDisclosure key={group.key} label={group.label} calls={group.calls} active={active} />
      ))}
    </div>
  )
}
