import { LoaderCircle } from 'lucide-react'
import { memo, useState } from 'react'
import { Collapse } from '@/components/ui/Collapse'
import { DisclosureTrigger } from '@/components/ui/DisclosureTrigger'
import { useInlineDiffs, useInlineShellCommands } from '@/lib/appearance'
import { ACPEventHeader } from './ACPEventHeader'
import { EditDiffBlock, hasInlineDiff } from './EditDiffBlock'
import { ShellCommandBlock, hasInlineShellCommand } from './ShellCommandBlock'
import { ThinkingDetail } from './ThinkingDetail'
import { ToolCallDetail } from './ToolCallContent'
import { hasToolCallDetail, toolRunLabel } from './toolPresentation'
import type { ActivityEntry, ActivityHeader } from './timeline'

const ActivityRunDisclosure = memo(function ActivityRunDisclosure({
  entries,
  active = false,
  findActive = false,
}: {
  entries: ActivityEntry[]
  active?: boolean
  findActive?: boolean
}) {
  const [open, setOpen] = useState(false)
  const calls = entries.flatMap((entry) => entry.kind === 'tool' ? [entry.call] : [])
  const details = entries.filter(
    (entry) => entry.kind === 'thought' ? Boolean(entry.text.trim()) : hasToolCallDetail(entry.call),
  )
  const expandable = details.length > 0
  const effectiveOpen = expandable && (open || findActive)
  const label = calls.length ? toolRunLabel(calls) : active ? 'Thinking' : 'Thought process'
  return (
    <div className="flex w-full flex-col items-start">
      <DisclosureTrigger
        label={label}
        open={effectiveOpen}
        disabled={!expandable}
        onClick={() => setOpen((value) => !value)}
        accessory={active ? (
          <LoaderCircle className="size-3 animate-spin text-running" aria-hidden />
        ) : undefined}
      />
      <Collapse open={effectiveOpen} className="w-full">
        <div className="relative w-full py-0.5 before:absolute before:bottom-4 before:left-[9px] before:top-4 before:w-px before:bg-border/75">
          {details.map((entry) =>
            entry.kind === 'thought' ? (
              <ThinkingDetail key={entry.key} text={entry.text} />
            ) : (
              <ToolCallDetail key={entry.key} call={entry.call} active={active} />
            ),
          )}
        </div>
      </Collapse>
    </div>
  )
})

export const ActivityDisclosure = memo(function ActivityDisclosure({
  entries,
  header,
  active = false,
  findActive = false,
}: {
  entries: ActivityEntry[]
  header?: ActivityHeader
  active?: boolean
  findActive?: boolean
}) {
  const inlineDiffs = useInlineDiffs()
  const inlineShell = useInlineShellCommands()
  const rows: (
    | { kind: 'run'; entries: ActivityEntry[]; key: string }
    | { kind: 'diff'; entry: Extract<ActivityEntry, { kind: 'tool' }> }
    | { kind: 'shell'; entry: Extract<ActivityEntry, { kind: 'tool' }> }
  )[] = []
  let run: ActivityEntry[] = []
  const flushRun = () => {
    if (!run.length) return
    rows.push({ kind: 'run', entries: run, key: `run-${run[0].key}` })
    run = []
  }
  for (const entry of entries) {
    if (entry.kind === 'tool' && inlineDiffs && hasInlineDiff(entry.call)) {
      flushRun()
      rows.push({ kind: 'diff', entry })
      continue
    }
    if (entry.kind === 'tool' && inlineShell && hasInlineShellCommand(entry.call)) {
      flushRun()
      rows.push({ kind: 'shell', entry })
      continue
    }
    run.push(entry)
  }
  flushRun()

  return (
    <div className="flex w-full max-w-[var(--prose-max)] flex-col gap-1">
      {header ? <ACPEventHeader {...header} /> : null}
      {rows.map((row, index) => {
        const rowActive = active && index === rows.length - 1
        switch (row.kind) {
          case 'run':
            return (
              <ActivityRunDisclosure
                key={row.key}
                entries={row.entries}
                active={rowActive}
                findActive={findActive}
              />
            )
          case 'diff':
            return <EditDiffBlock key={row.entry.key} call={row.entry.call} />
          case 'shell':
            return (
              <ShellCommandBlock
                key={row.entry.key}
                call={row.entry.call}
                active={rowActive}
              />
            )
        }
      })}
    </div>
  )
})
