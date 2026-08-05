import {
  CircleEllipsis,
  FilePenLine,
  FolderOpen,
  Globe,
  Image,
  LoaderCircle,
  Search,
  SquareTerminal,
  type LucideIcon,
} from 'lucide-react'
import { memo, useState } from 'react'
import { Collapse } from '@/components/ui/Collapse'
import { DisclosureTrigger } from '@/components/ui/DisclosureTrigger'
import { useInlineDiffs, useInlineShellCommands } from '@/lib/appearance'
import { ACPEventHeader } from './ACPEventHeader'
import { EditDiffBlock, hasInlineDiff } from './EditDiffBlock'
import { MessageMarkdown } from './MessageMarkdown'
import { ShellCommandBlock, hasInlineShellCommand } from './ShellCommandBlock'
import { ToolCallDetail } from './ToolCallContent'
import {
  hasToolCallDetail,
  toolCallCategory,
  toolRunLabel,
  type ToolCategory,
} from './toolPresentation'
import type { ActivityEntry, ActivityHeader } from './timeline'

type ToolEntry = Extract<ActivityEntry, { kind: 'tool' }>

export const ACPThought = memo(function ACPThought({ text }: { text: string }) {
  return (
    <div className="min-w-0 py-1 text-pretty select-text">
      <MessageMarkdown text={text} />
    </div>
  )
})

function toolRunIcon(categories: ToolCategory[]): LucideIcon {
  if (categories.includes('edit')) return FilePenLine
  if (categories.includes('read')) return FolderOpen
  if (categories.includes('command')) return SquareTerminal
  if (categories.includes('web_fetch') || categories.includes('web_search')) return Globe
  if (categories.includes('search')) return Search
  if (categories.includes('image')) return Image
  return CircleEllipsis
}

const ActivityToolDisclosure = memo(function ActivityToolDisclosure({
  entries,
  active = false,
  findActive = false,
}: {
  entries: ToolEntry[]
  active?: boolean
  findActive?: boolean
}) {
  const [open, setOpen] = useState(false)
  const calls = entries.map((entry) => entry.call)
  const details = entries.filter((entry) => hasToolCallDetail(entry.call))
  const expandable = details.length > 0
  const effectiveOpen = expandable && (open || findActive)
  const Icon = toolRunIcon(calls.map(toolCallCategory))
  return (
    <div className="flex w-full flex-col items-start">
      <DisclosureTrigger
        label={(
          <span className="flex min-w-0 items-center gap-2">
            <Icon className="size-3.5 shrink-0" aria-hidden />
            <span className="truncate">{toolRunLabel(calls)}</span>
          </span>
        )}
        open={effectiveOpen}
        disabled={!expandable}
        onClick={() => setOpen((value) => !value)}
        accessory={active ? (
          <LoaderCircle className="size-3 animate-spin text-running" aria-hidden />
        ) : undefined}
      />
      <Collapse open={effectiveOpen} className="w-full">
        <div className="relative w-full py-0.5 before:absolute before:bottom-4 before:left-[9px] before:top-4 before:w-px before:bg-border/75">
          {details.map((entry) => (
            <ToolCallDetail key={entry.key} call={entry.call} active={active} />
          ))}
        </div>
      </Collapse>
    </div>
  )
})

export const ActivityBlock = memo(function ActivityBlock({
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
    | { kind: 'thought'; entry: Extract<ActivityEntry, { kind: 'thought' }> }
    | { kind: 'tools'; entries: ToolEntry[]; key: string }
    | { kind: 'diff'; entry: ToolEntry }
    | { kind: 'shell'; entry: ToolEntry }
  )[] = []
  let tools: ToolEntry[] = []
  const flushTools = () => {
    if (!tools.length) return
    rows.push({ kind: 'tools', entries: tools, key: `tools-${tools[0].key}` })
    tools = []
  }
  for (const entry of entries) {
    if (entry.kind === 'thought') {
      flushTools()
      rows.push({ kind: 'thought', entry })
      continue
    }
    if (inlineDiffs && hasInlineDiff(entry.call)) {
      flushTools()
      rows.push({ kind: 'diff', entry })
      continue
    }
    if (inlineShell && hasInlineShellCommand(entry.call)) {
      flushTools()
      rows.push({ kind: 'shell', entry })
      continue
    }
    tools.push(entry)
  }
  flushTools()

  return (
    <div className="flex w-full max-w-[var(--prose-max)] flex-col gap-2">
      {header ? <ACPEventHeader {...header} /> : null}
      {rows.map((row, index) => {
        const rowActive = active && index === rows.length - 1
        switch (row.kind) {
          case 'thought':
            return <ACPThought key={row.entry.key} text={row.entry.text} />
          case 'tools':
            return (
              <ActivityToolDisclosure
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
