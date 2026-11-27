import {
  Bot,
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
import { memo, useLayoutEffect, useRef, useState, type ReactElement } from 'react'
import { Collapse } from '@/components/ui/Collapse'
import { DisclosureTrigger } from '@/components/ui/DisclosureTrigger'
import { useInlineDiffs, useInlineShellCommands } from '@/lib/appearance'
import { ACPEventHeader } from './ACPEventHeader'
import { AppDraftCard } from './AppDraftCard'
import { isAppDraftToolCall } from './appDraftTool'
import { EditDiffBlock, hasInlineDiff } from './EditDiffBlock'
import { ThinkingBlock } from '@/components/session/ThinkingBlock'
import { ShellCommandBlock, hasInlineShellCommand } from './ShellCommandBlock'
import { ToolCallDetail } from './ToolCallContent'
import { ToolActivityReveal } from '@/components/session/ToolActivityReveal'
import {
  hasToolCallDetail,
  isRunningToolStatus,
  toolCallCategory,
  toolRunLabel,
  type ToolCategory,
} from './toolPresentation'
import type { ActivityEntry, ActivityHeader } from './timeline'

type ToolEntry = Extract<ActivityEntry, { kind: 'tool' }>

function toolRunIcon(categories: ToolCategory[]): LucideIcon {
  if (categories.includes('edit')) return FilePenLine
  if (categories.includes('read')) return FolderOpen
  if (categories.includes('command')) return SquareTerminal
  if (categories.includes('web_fetch') || categories.includes('web_search')) return Globe
  if (categories.includes('search')) return Search
  if (categories.includes('image')) return Image
  if (categories.includes('agent')) return Bot
  return CircleEllipsis
}

const ActivityToolDisclosure = memo(function ActivityToolDisclosure({
  entries,
  active = false,
  findActive = false,
  revealTools,
}: {
  entries: ToolEntry[]
  active?: boolean
  findActive?: boolean
  revealTools?: ReadonlySet<string>
}) {
  const [open, setOpen] = useState(false)
  const calls = entries.map((entry) => entry.call)
  const label = toolRunLabel(calls)
  const labelRef = useRef<HTMLSpanElement>(null)
  const previous = useRef<{ entries: ToolEntry[]; label: string }>(null)
  const last = previous.current
  const append = revealTools && last && entries.length > last.entries.length &&
    label.startsWith(`${last.label}, `) &&
    last.entries.every((entry, index) => entry.key === entries[index].key) &&
    entries.slice(last.entries.length).every((entry) => revealTools.has(entry.key))
    ? { labelRef, from: last.label.length }
    : undefined
  useLayoutEffect(() => {
    previous.current = { entries, label }
  }, [entries, label])

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
            <span ref={labelRef} className="relative min-w-0">
              <span className="block truncate">{label}</span>
            </span>
          </span>
        )}
        open={effectiveOpen}
        disabled={!expandable}
        onClick={() => setOpen((value) => !value)}
        wrapContent={(content) => (
          <ToolActivityReveal
            reveal={revealTools && entries.every((entry) => revealTools.has(entry.key))}
            append={append}
            className="flex min-w-0 items-center gap-1.5"
          >
            {content}
          </ToolActivityReveal>
        )}
        accessory={active && calls.some((call) => isRunningToolStatus(call.status)) ? (
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
  revealTools,
}: {
  entries: ActivityEntry[]
  header?: ActivityHeader
  active?: boolean
  findActive?: boolean
  revealTools?: ReadonlySet<string>
}) {
  const inlineDiffs = useInlineDiffs()
  const inlineShell = useInlineShellCommands()
  const rows: ReactElement[] = []
  let tools: ToolEntry[] = []
  const flushTools = (rowActive = false) => {
    if (!tools.length) return
    rows.push(
      <ActivityToolDisclosure
        key={`tools-${tools[0].key}`}
        entries={tools}
        active={rowActive}
        findActive={findActive}
        revealTools={revealTools}
      />,
    )
    tools = []
  }

  for (const [index, entry] of entries.entries()) {
    const rowActive = active && index === entries.length - 1
    let row: ReactElement
    if (entry.kind === 'thought') {
      row = <ThinkingBlock key={entry.key} text={entry.text} pending={rowActive} findActive={findActive} />
    } else if (isAppDraftToolCall(entry.call)) {
      row = <AppDraftCard key={entry.key} call={entry.call} active={rowActive} />
    } else if (inlineDiffs && hasInlineDiff(entry.call)) {
      row = <EditDiffBlock key={entry.key} call={entry.call} />
    } else if (inlineShell && hasInlineShellCommand(entry.call)) {
      row = <ShellCommandBlock key={entry.key} call={entry.call} active={rowActive} reveal={revealTools?.has(entry.key)} />
    } else {
      tools.push(entry)
      continue
    }
    flushTools()
    rows.push(row)
  }
  flushTools(active)

  return (
    <div className="flex w-full max-w-[var(--prose-max)] flex-col gap-2">
      {header ? <ACPEventHeader {...header} /> : null}
      {rows}
    </div>
  )
})
