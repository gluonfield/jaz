import { motion } from 'motion/react'
import { Link } from '@tanstack/react-router'
import {
  Check,
  ChevronDown,
  ChevronRight,
  Circle,
  CircleCheck,
  FileText,
  LoaderCircle,
} from 'lucide-react'
import { memo, useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import type {
  ACPMeta,
  ACPPermission,
  ACPToolCall,
  ChatMessage,
  MessageBlock,
  SessionEvent,
} from '@/lib/api/types'
import { Button } from '@/components/ui/Button'
import { IconButton } from '@/components/ui/IconButton'
import { agentLabel } from '@/lib/agentLabel'
import { relativeTime } from '@/lib/format/time'
import {
  planStepState,
  planSurfaceFromEvent,
  planSurfaceKey,
  type PlanStepState,
  type PlanSurface,
} from '@/lib/planSurface'
import { MessageMarkdown } from './MessageMarkdown'
import { ThinkingBlock } from './ThinkingBlock'
import { PermissionCard } from './TranscriptPermissions'
import { hasPermissionSurface, normalized } from './TranscriptUtils'
import { ToolCallCard } from './ToolCallCard'
import { isHiddenToolName } from './toolVisibility'

function messageText(message: ChatMessage): string {
  // Each text block is a separate utterance; join as paragraphs so block
  // boundaries don't fuse sentences together ("…intact.Updated…").
  const text = message.blocks
    ?.filter((block) => block.type === 'text')
    .map((block) => (block.text ?? '').trim())
    .filter(Boolean)
    .join('\n\n')
  return text || message.content
}

function messageReasoning(message: ChatMessage): string {
  const text = message.blocks
    ?.filter((block) => block.type === 'reasoning')
    .map((block) => (block.text ?? '').trim())
    .filter(Boolean)
    .join('\n\n')
  return text || message.reasoning || ''
}

function isVisibleToolBlock(block: MessageBlock): block is Extract<MessageBlock, { type: 'tool' }> {
  return block.type === 'tool' && !isHiddenToolName(block.name)
}

function formatAttachmentSize(size?: number): string {
  if (!size) return ''
  if (size < 1024) return `${size} B`
  if (size < 1024 * 1024) return `${Math.round(size / 1024)} KB`
  return `${(size / (1024 * 1024)).toFixed(1)} MB`
}

function MessageAttachments({ message }: { message: ChatMessage }) {
  const attachments = message.blocks?.filter((block) => block.type === 'attachment') ?? []
  if (!attachments.length) return null
  return (
    <div className="mt-2 flex flex-wrap gap-1">
      {attachments.map((attachment) => (
        <span
          key={attachment.id}
          className="inline-flex max-w-full items-center gap-1.5 rounded-full bg-bg px-2.5 py-1 text-xs text-ink-2"
          title={attachment.server_path ?? attachment.uri}
        >
          <FileText size={13} className="shrink-0 text-primary" />
          <span className="max-w-[220px] truncate text-ink">{attachment.name}</span>
          <span className="shrink-0 text-ink-3">{formatAttachmentSize(attachment.size)}</span>
        </span>
      ))}
    </div>
  )
}

const Bubble = memo(function Bubble({ message }: { message: ChatMessage }) {
  switch (message.role) {
    case 'user':
      return (
        <div className="flex justify-end">
          <div className="max-w-[80%] rounded-card bg-surface px-3.5 py-2.5 text-sm whitespace-pre-wrap select-text">
            {messageText(message)}
            <MessageAttachments message={message} />
          </div>
        </div>
      )
    case 'assistant': {
      const text = messageText(message)
      const reasoning = messageReasoning(message)
      return (
        <div className="flex max-w-[72ch] flex-col gap-2">
          <ThinkingBlock text={reasoning} />
          {text ? <MessageMarkdown text={text} /> : null}
          {message.blocks
            ?.filter(isVisibleToolBlock)
            .map((block) => (
              <ToolCallCard
                key={block.id}
                name={block.name}
                args={block.input_json}
                result={block.result}
                pending={block.result === undefined || block.result === ''}
              />
          ))}
        </div>
      )
    }
    // system/developer prompts are plumbing, not conversation — never shown
    default:
      return null
  }
})

function isParentChildACPEvent(event: SessionEvent): boolean {
  return Boolean(
    event.acp?.parent_id &&
      event.acp.parent_id === event.session_id &&
      event.acp.id !== event.session_id,
  )
}

function hasWorkingStatusSurface(event: SessionEvent): boolean {
  return Boolean(event.acp && normalized(event.acp.state) === 'running')
}

// A status event whose only surface is the "working on …" link.
function isWorkingLinkOnly(event: SessionEvent): boolean {
  return (
    event.type === 'acp' &&
    hasWorkingStatusSurface(event) &&
    !planSurfaceFromEvent(event) &&
    !event.content &&
    !event.acp?.thought &&
    !event.acp?.error &&
    !event.acp?.tool_calls?.length
  )
}

function hasVisibleACPSurface(event: SessionEvent): boolean {
  const acp = event.acp
  if (!acp) return false
  const hasPlan = Boolean(planSurfaceFromEvent(event))
  if (isParentChildACPEvent(event)) {
    return Boolean(
      event.content ||
        acp.thought ||
        acp.error ||
        hasPlan ||
        hasWorkingStatusSurface(event),
    )
  }
  return Boolean(
    event.content ||
      acp.thought ||
      acp.error ||
      acp.tool_calls?.length ||
      hasPlan ||
      hasWorkingStatusSurface(event),
  )
}


function childLabel(event: SessionEvent, meta?: ACPMeta): string {
  const acp = event.acp
  const named = acp ? meta?.[acp.id] : undefined
  return acp?.title || named?.title || acp?.slug || named?.slug || 'child task'
}

interface ToolGroup {
  key: string
  label: string
  calls: ACPToolCall[]
}

function toolGroupKey(call: ACPToolCall): string {
  const title = call.title ?? call.id
  if (/^edit\s/i.test(title)) return 'edit'
  if (/^read\s/i.test(title)) return 'read'
  if (/^search\s/i.test(title)) return 'search'
  if (/^view image\s/i.test(title)) return 'image'
  if (/^(command\s+-v|npx\s|npm\s|bun\s|go\s|git\s|python3?\s|tidy\s|wc\s|rg\s)/i.test(title)) return 'command'
  return 'tool'
}

function toolGroupBaseLabel(key: string, count: number): string {
  const plural = count === 1 ? '' : 's'
  switch (key) {
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
  const order = ['edit', 'read', 'search', 'image', 'command', 'tool']
  const byKey = new Map<string, ACPToolCall[]>()
  for (const call of calls) {
    const key = toolGroupKey(call)
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

// One codex-style phrase for a run of tool calls: "Explored 2 files, ran 1 command".
function toolRunLabel(calls: ACPToolCall[]): string {
  const phrases: Record<string, (n: number) => string> = {
    edit: (n) => `edited ${n} file${n === 1 ? '' : 's'}`,
    read: (n) => `explored ${n} file${n === 1 ? '' : 's'}`,
    search: (n) => `searched ${n} time${n === 1 ? '' : 's'}`,
    image: (n) => `viewed ${n} image${n === 1 ? '' : 's'}`,
    command: (n) => `ran ${n} command${n === 1 ? '' : 's'}`,
    tool: (n) => `used ${n} tool${n === 1 ? '' : 's'}`,
  }
  const order = ['read', 'search', 'command', 'edit', 'image', 'tool']
  const counts = new Map<string, number>()
  let failed = 0
  for (const call of calls) {
    const key = toolGroupKey(call)
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

const ToolDisclosure = memo(function ToolDisclosure({
  label,
  calls,
  active = false,
}: {
  label: string
  calls: ACPToolCall[]
  active?: boolean
}) {
  const [open, setOpen] = useState(false)
  // Old sessions can hold stale non-terminal statuses; only spin while the
  // session is actually working.
  const running =
    active &&
    calls.some((call) =>
      ['pending', 'in_progress', 'in-progress', 'running'].includes(normalized(call.status)),
    )
  return (
    <div className="flex flex-col items-start gap-1">
      <button
        type="button"
        aria-expanded={open}
        onClick={() => setOpen((value) => !value)}
        className="inline-flex min-h-7 items-center gap-1.5 rounded-full px-1 text-left font-mono text-[12px] text-ink-3 transition-colors hover:text-ink"
      >
        <ChevronRight
          size={12}
          className={`shrink-0 transition-transform ${open ? 'rotate-90' : ''}`}
          aria-hidden
        />
        {label}
        {running ? (
          <LoaderCircle className="size-3 animate-spin text-running" aria-hidden />
        ) : null}
      </button>
      {open ? (
        <div className="ml-4 flex max-w-full flex-col gap-1">
          {calls.map((call) => (
            <span
              key={call.id}
              className="max-w-full rounded border border-border bg-bg px-1.5 py-px font-mono text-[11px] whitespace-pre-wrap text-ink-2"
            >
              {call.title || call.id}
              {call.status ? (
                <span className="text-ink-3"> · {call.status}</span>
              ) : null}
            </span>
          ))}
        </div>
      ) : null}
    </div>
  )
})

function ToolSummary({ calls, active = false }: { calls?: ACPToolCall[]; active?: boolean }) {
  if (!calls?.length) return null
  return (
    <div className="flex flex-col items-start gap-1.5">
      {groupToolCalls(calls).map((group) => (
        <ToolDisclosure key={group.key} label={group.label} calls={group.calls} active={active} />
      ))}
    </div>
  )
}

export function PlanStepIcon({ state, active }: { state: PlanStepState; active: boolean }) {
  switch (state) {
    case 'completed':
      return <CircleCheck size={14} className="text-ok" aria-hidden />
    case 'active':
      return (
        <LoaderCircle
          size={14}
          className={`text-running ${active ? 'animate-spin' : ''}`}
          aria-hidden
        />
      )
    default:
      return <Circle size={14} className="text-ink-3" aria-hidden />
  }
}

const PlanChecklist = memo(function PlanChecklist({
  surface,
  active = false,
  onApprovePlan,
}: {
  surface: PlanSurface
  active?: boolean
  onApprovePlan?: () => void
}) {
  const [expanded, setExpanded] = useState(false)
  const [overflowing, setOverflowing] = useState(false)
  const contentRef = useRef<HTMLDivElement>(null)
  const { title, explanation, entries, strikeCompleted } = surface

  useEffect(() => {
    const el = contentRef.current
    if (!el) return

    const measure = () => {
      setOverflowing(el.scrollHeight > el.clientHeight + 2)
    }
    measure()

    const observer = new ResizeObserver(measure)
    observer.observe(el)
    return () => observer.disconnect()
  }, [entries, expanded, explanation])

  const showExpandControl = expanded || overflowing
  const planEntries = entries ?? []
  const explanationText = explanation?.trim() ?? ''
  const stepStates = planEntries.map(planStepState)
  const showSteps = stepStates.some(Boolean)
  const completedCount = stepStates.filter((state) => state === 'completed').length

  return (
    <div className="rounded-card border border-border bg-surface/60 px-3 py-2.5">
      <div className="mb-2 flex items-center justify-between gap-3">
        <p className="text-[11px] font-medium tracking-wide text-ink-3 uppercase">
          {title}
          {showSteps ? (
            <span className="ml-2 font-mono normal-case tracking-normal">
              {completedCount}/{planEntries.length}
            </span>
          ) : null}
        </p>
        {surface.awaitingApproval && onApprovePlan ? (
          <Button variant="primary" size="sm" onClick={onApprovePlan}>
            <Check size={13} />
            Approve plan
          </Button>
        ) : null}
      </div>
      <div
        ref={contentRef}
        className={`relative ${expanded ? '' : 'max-h-[340px] overflow-hidden'}`}
      >
        {explanationText ? (
          <div className="mb-2 text-sm text-ink-2">
            <MessageMarkdown text={explanationText} />
          </div>
        ) : null}
        {planEntries.length ? (
          <ul className="flex flex-col gap-2.5">
            {planEntries.map((entry, index) => {
              const state = stepStates[index]
              const done = state === 'completed'
              return (
                <li
                  key={`${entry.content}-${index}`}
                  className="flex min-w-0 items-start gap-2 text-sm text-ink-2"
                >
                  {showSteps ? (
                    <span className="mt-[3px] shrink-0" title={state}>
                      <PlanStepIcon state={state ?? 'pending'} active={active} />
                    </span>
                  ) : null}
                  <div
                    className={`min-w-0 flex-1 ${done ? `opacity-50 ${strikeCompleted ? 'line-through' : ''}` : ''}`}
                  >
                    <MessageMarkdown text={entry.content} />
                  </div>
                </li>
              )
            })}
          </ul>
        ) : explanationText ? null : (
          <p className="text-sm italic text-ink-3">(no steps provided)</p>
        )}
        {!expanded && overflowing ? (
          <div
            className="pointer-events-none absolute inset-x-0 bottom-0 h-20 bg-gradient-to-b from-transparent via-surface/85 to-surface"
            aria-hidden
          />
        ) : null}
      </div>
      {/* Centered chevron-in-a-circle is the only expand affordance; it lifts
          onto the fade when collapsed and flips to point up when open. */}
      {showExpandControl ? (
        <div className={`relative z-10 flex justify-center ${expanded ? 'mt-1.5' : '-mt-3.5'}`}>
          <IconButton
            variant="ghost"
            size="md"
            aria-expanded={expanded}
            aria-label={expanded ? 'Collapse plan' : 'Expand plan'}
            title={expanded ? 'Collapse plan' : 'Expand plan'}
            className="border border-border bg-surface shadow-sm"
            onClick={() => setExpanded((value) => !value)}
          >
            <ChevronDown
              size={15}
              className={`transition-transform duration-200 ease-out ${expanded ? 'rotate-180' : ''}`}
              aria-hidden
            />
          </IconButton>
        </div>
      ) : null}
    </div>
  )
})

const LiveEvent = memo(function LiveEvent({
  event,
  sessionId,
  acpMeta,
  showHeader,
  working = false,
  permissionResolution,
  showPlan,
  onApprovePlan,
}: {
  event: SessionEvent
  sessionId?: string
  acpMeta?: ACPMeta
  showHeader: boolean
  working?: boolean
  permissionResolution?: ACPPermission
  showPlan?: boolean
  onApprovePlan?: () => void
}) {
  const eventPlan = planSurfaceFromEvent(event)
  const planSurface = showPlan ? eventPlan : undefined
  const headerTitle = event.acp ? event.acp.title || acpMeta?.[event.acp.id]?.title : undefined
  const ownSession = Boolean(event.acp && event.acp.id === sessionId)
  const showWorkingStatus =
    event.type === 'acp' && hasWorkingStatusSurface(event) && !eventPlan && !ownSession
  const parentChild = isParentChildACPEvent(event)
  return (
    <motion.div
      className="flex max-w-[72ch] flex-col gap-2"
      initial={{ opacity: 0, y: 6 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ type: 'spring', stiffness: 380, damping: 30 }}
    >
      {event.acp && showHeader ? (
        <p className="text-[12px] text-ink-3">
          <span className="font-mono">{event.acp.agent}</span>
          {headerTitle ? ` · ${headerTitle}` : ''} · {relativeTime(event.at)}
        </p>
      ) : null}
      {event.acp?.thought ? <ThinkingBlock text={event.acp.thought} /> : null}
      {event.content ? <MessageMarkdown text={event.content} /> : null}
      {event.acp?.error ? (
        <p className="rounded-card bg-danger-soft px-3 py-2 text-sm text-danger select-text">
          {event.acp.error}
        </p>
      ) : null}
      {showWorkingStatus ? (
        <Link
          to="/sessions/$sessionId"
          params={{ sessionId: event.acp?.id ?? event.session_id }}
          className="inline-flex w-fit items-center gap-2 rounded-card border border-border bg-surface px-3 py-2 text-sm text-ink-2 transition-colors hover:border-primary hover:text-primary"
        >
          <LoaderCircle className="size-4 animate-spin text-running" aria-hidden />
          <span>{agentLabel(event.acp?.agent)} is working on {childLabel(event, acpMeta)}</span>
        </Link>
      ) : null}
      {!parentChild ? <ToolSummary calls={event.acp?.tool_calls} active={working} /> : null}
      {event.permission ? (
        <PermissionCard event={event} resolution={permissionResolution} />
      ) : null}
      {planSurface ? (
        <PlanChecklist surface={planSurface} active={working} onApprovePlan={onApprovePlan} />
      ) : null}
    </motion.div>
  )
})

type TimelineItem =
  | { kind: 'message'; message: ChatMessage; at: number }
  | { kind: 'event'; event: SessionEvent; eventIndex: number; at: number; showHeader: boolean }
  | { kind: 'tools'; calls: ACPToolCall[]; at: number; key: string }

function itemTime(value: string | undefined): number {
  const parsed = Date.parse(value ?? '')
  return Number.isNaN(parsed) ? 0 : parsed
}

// Head-only merge of two ordered streams preserves each stream's internal
// order even when timestamps are unreliable.
function mergeTimeline(
  messages: ChatMessage[],
  events: { event: SessionEvent; index: number }[],
): TimelineItem[] {
  const out: TimelineItem[] = []
  let i = 0
  let j = 0
  while (i < messages.length || j < events.length) {
    const message = messages[i]
    const entry = events[j]
    if (!entry || (message && itemTime(message.created_at) <= itemTime(entry.event.at))) {
      out.push({ kind: 'message', message, at: itemTime(message.created_at) })
      i += 1
    } else {
      out.push({
        kind: 'event',
        event: entry.event,
        eventIndex: entry.index,
        at: itemTime(entry.event.at),
        showHeader: false,
      })
      j += 1
    }
  }
  return out
}

// Consecutive tool events collapse into one run: "Explored 2 files, ran 1 command".
function groupToolRuns(items: TimelineItem[]): TimelineItem[] {
  const out: TimelineItem[] = []
  for (const item of items) {
    const isToolEvent =
      item.kind === 'event' &&
      item.event.type === 'acp_tool' &&
      item.event.acp?.tool_calls?.length === 1
    if (!isToolEvent) {
      out.push(item)
      continue
    }
    const call = (item as Extract<TimelineItem, { kind: 'event' }>).event.acp!.tool_calls![0]
    const prev = out.at(-1)
    if (prev?.kind === 'tools') {
      const existing = prev.calls.findIndex((candidate) => candidate.id === call.id)
      if (existing === -1) prev.calls.push(call)
      else prev.calls[existing] = call
      prev.at = item.at
      continue
    }
    out.push({ kind: 'tools', calls: [call], at: item.at, key: `tools-${call.id}` })
  }
  return out
}

function markEventHeaders(items: TimelineItem[], sessionId?: string): void {
  let previousACP = ''
  for (const item of items) {
    if (item.kind !== 'event') {
      if (item.kind === 'message') previousACP = ''
      continue
    }
    const acp = item.event.acp
    if (!acp) {
      previousACP = ''
      continue
    }
    // Own-page events need no byline; a child agent's run is introduced once.
    item.showHeader = Boolean(sessionId && acp.id !== sessionId && acp.id !== previousACP)
    previousACP = acp.id
  }
}

function formatDuration(ms: number): string {
  const totalSeconds = Math.max(1, Math.round(ms / 1000))
  const hours = Math.floor(totalSeconds / 3600)
  const minutes = Math.floor((totalSeconds % 3600) / 60)
  const seconds = totalSeconds % 60
  if (hours) return `${hours}h ${minutes}m`
  if (minutes) return `${minutes}m ${seconds}s`
  return `${seconds}s`
}

interface Turn {
  opener?: TimelineItem
  items: TimelineItem[]
}

function splitTurns(items: TimelineItem[]): Turn[] {
  const turns: Turn[] = []
  for (const item of items) {
    if (item.kind === 'message' && item.message.role === 'user') {
      turns.push({ opener: item, items: [] })
      continue
    }
    if (!turns.length) turns.push({ items: [] })
    turns.at(-1)!.items.push(item)
  }
  return turns
}

// Work that may fold under "Worked for Xs"; plans, pending questions, and errors stay out.
function isCollapsibleWork(
  item: TimelineItem,
  pendingPermissionIds: Set<string>,
  latestPlanIndex: Map<string, number>,
): boolean {
  if (item.kind === 'tools') return true
  if (item.kind !== 'event') return false
  const event = item.event
  if (event.type === 'acp_thought') return true
  if (event.type === 'acp_message') return true
  if (event.type === 'permission_request') {
    return !pendingPermissionIds.has(event.permission?.id ?? '')
  }
  const planSurface = planSurfaceFromEvent(event)
  if (planSurface) {
    return event.acp ? latestPlanIndex.get(event.acp.id) !== item.eventIndex : false
  }
  if (event.type === 'acp') {
    if (event.acp?.error) return false
    return true
  }
  return false
}

function WorkSection({
  items,
  durationMs,
  defaultOpen,
  render,
}: {
  items: TimelineItem[]
  durationMs: number
  defaultOpen: boolean
  render: (item: TimelineItem) => ReactNode
}) {
  const [open, setOpen] = useState(defaultOpen)
  return (
    <div className="flex flex-col gap-5">
      <button
        type="button"
        aria-expanded={open}
        onClick={() => setOpen((value) => !value)}
        className="inline-flex min-h-7 items-center gap-1.5 self-start rounded-full px-1 text-left text-[12px] font-medium text-ink-3 transition-colors hover:text-ink"
      >
        <ChevronRight
          size={12}
          className={`shrink-0 transition-transform ${open ? 'rotate-90' : ''}`}
          aria-hidden
        />
        Worked for {formatDuration(durationMs)}
      </button>
      {open ? (
        <div className="flex flex-col gap-5 border-l border-border pl-4">
          {items.map((item) => render(item))}
        </div>
      ) : null}
    </div>
  )
}

// Everything between raw history and JSX that doesn't depend on render-only
// state (working, tail). The component memoizes one call per data change so
// parent renders and streaming flags don't rebuild the timeline.
function buildTimeline(
  messages: ChatMessage[],
  events: SessionEvent[],
  sessionId: string | undefined,
  groupTurns: boolean,
) {
  const combinedEvents = combineSequentialACPText(events)
  const permissionResolutions = new Map<string, ACPPermission>()
  const latestPermissionRequest = new Map<string, number>()
  const latestPlanEvent = new Map<string, number>()
  const latestToolEvent = new Map<string, number>()
  combinedEvents.forEach((event, index) => {
    if (event.type === 'permission_request' && event.permission) {
      latestPermissionRequest.set(event.permission.id, index)
    }
    if (event.type === 'permission_response' && event.permission) {
      permissionResolutions.set(event.permission.id, event.permission)
    }
    const acp = event.acp
    if (acp && planSurfaceFromEvent(event)) {
      latestPlanEvent.set(acp.id, index)
    }
    if (event.type === 'acp' && acp?.tool_calls?.length) {
      latestToolEvent.set(acp.id, index)
    }
  })
  const renderedEvents = combinedEvents
    .map((event, index) => ({ event, index }))
    .filter(({ event, index }) => {
      if (event.type === 'permission_response' || event.type === 'assistant') return false
      if (event.type === 'permission_request' && !hasPermissionSurface(event.permission)) {
        return false
      }
      if (
        event.type === 'permission_request' &&
        event.permission?.id &&
        latestPermissionRequest.get(event.permission.id) !== index
      ) {
        return false
      }
      const acp = event.acp
      const planSurface = planSurfaceFromEvent(event)
      if (!acp) {
        if (planSurface) return true
        return Boolean(event.content || event.permission)
      }
      if (!hasVisibleACPSurface(event)) return false
      // This page's own running state has no link to render — drop the event
      // instead of leaving an empty row.
      if (isWorkingLinkOnly(event) && acp.id === sessionId) return false
      if (planSurface) {
        const isLatestPlan = latestPlanEvent.get(acp.id) === index
        if (!isLatestPlan && !event.content && !acp.error && !acp.tool_calls?.length) return false
      }
      if (event.type === 'acp' && acp.tool_calls?.length && latestToolEvent.get(acp.id) !== index) {
        return Boolean(event.content || acp.error || planSurface)
      }
      return true
    })

  const pendingPermissionIds = new Set<string>()
  for (const event of combinedEvents) {
    if (event.type === 'permission_request' && event.permission?.id) {
      pendingPermissionIds.add(event.permission.id)
    }
    if (event.type === 'permission_response' && event.permission?.id) {
      pendingPermissionIds.delete(event.permission.id)
    }
  }

  const visibleMessages = messages.filter((message) => message.role === 'user' || message.role === 'assistant')
  const merged = groupToolRuns(mergeTimeline(visibleMessages, renderedEvents))
  // Live state isn't history: pending questions and working status anchor at
  // the bottom; an answered question returns to its chronological spot.
  const isPendingCard = (item: TimelineItem) =>
    item.kind === 'event' &&
    item.event.type === 'permission_request' &&
    pendingPermissionIds.has(item.event.permission?.id ?? '')
  const isWorkingLink = (item: TimelineItem) =>
    item.kind === 'event' && isWorkingLinkOnly(item.event)
  const pendingCards = merged.filter(isPendingCard)
  const workingLinks = merged.filter(isWorkingLink)
  const chronological = merged.filter((item) => !isPendingCard(item) && !isWorkingLink(item))
  // "Working on …" next to a question that's waiting for the user is noise.
  const anchored = [...(pendingCards.length ? [] : workingLinks), ...pendingCards]
  markEventHeaders([...chronological, ...anchored], sessionId)
  return {
    chronological,
    anchored,
    turns: groupTurns ? splitTurns(chronological) : [],
    permissionResolutions,
    latestPlanEvent,
    pendingPermissionIds,
  }
}

export const Transcript = memo(function Transcript({
  messages,
  events,
  sessionId,
  acpMeta,
  groupTurns = false,
  working = false,
  tail,
  onApprovePlan,
}: {
  messages: ChatMessage[]
  events: SessionEvent[]
  sessionId?: string
  acpMeta?: ACPMeta
  groupTurns?: boolean
  working?: boolean
  // in-flight live exchange, rendered between history and anchored live state
  tail?: ReactNode
  onApprovePlan?: () => void
}) {
  const {
    chronological,
    anchored,
    turns,
    permissionResolutions,
    latestPlanEvent,
    pendingPermissionIds,
  } = useMemo(
    () => buildTimeline(messages, events, sessionId, groupTurns),
    [messages, events, sessionId, groupTurns],
  )

  const renderItem = (item: TimelineItem): ReactNode => {
    switch (item.kind) {
      case 'message':
        return <Bubble key={`message-${item.message.seq}`} message={item.message} />
      case 'tools':
        return (
          <ToolDisclosure
            key={item.key}
            label={toolRunLabel(item.calls)}
            calls={item.calls}
            active={working}
          />
        )
      case 'event': {
        const planSurface = planSurfaceFromEvent(item.event)
        return (
          <LiveEvent
            key={`event-${stableEventKey(item.event)}`}
            event={item.event}
            sessionId={sessionId}
            acpMeta={acpMeta}
            showHeader={item.showHeader}
            working={working}
            showPlan={
              Boolean(
                planSurface &&
                  (!item.event.acp || latestPlanEvent.get(item.event.acp.id) === item.eventIndex),
              )
            }
            onApprovePlan={onApprovePlan}
            permissionResolution={
              item.event.permission ? permissionResolutions.get(item.event.permission.id) : undefined
            }
          />
        )
      }
    }
  }

  if (!groupTurns) {
    return (
      <div className="flex flex-col gap-5">
        {chronological.map((item) => renderItem(item))}
        {tail}
        {anchored.map((item) => renderItem(item))}
      </div>
    )
  }

  return (
    <div className="flex flex-col gap-5">
      {turns.map((turn, turnIndex) => {
        const active = working && turnIndex === turns.length - 1
        const lastContentIndex = turn.items.findLastIndex(
          (item) => item.kind === 'event' && Boolean(item.event.content?.trim()),
        )
        const sections: ReactNode[] = []
        if (turn.opener) sections.push(renderItem(turn.opener))
        let work: TimelineItem[] = []
        const flushWork = () => {
          if (!work.length) return
          const batch = work
          work = []
          const durationMs =
            batch[batch.length - 1].at - (turn.opener?.at ?? batch[0].at)
          sections.push(
            <WorkSection
              key={`work-${turnIndex}-${sections.length}`}
              items={batch}
              durationMs={durationMs}
              defaultOpen={false}
              render={renderItem}
            />,
          )
        }
        turn.items.forEach((item, index) => {
          const collapsible =
            !active &&
            index < lastContentIndex &&
            isCollapsibleWork(item, pendingPermissionIds, latestPlanEvent)
          if (collapsible) {
            work.push(item)
            return
          }
          flushWork()
          sections.push(renderItem(item))
        })
        flushWork()
        return (
          <div key={`turn-${turnIndex}`} className="flex flex-col gap-5">
            {sections}
          </div>
        )
      })}
      {tail}
      {anchored.map((item) => renderItem(item))}
    </div>
  )
})

// Coalesced events keep their latest copy whose seq changes per update; key by
// identity so streamed deltas patch in place instead of remounting.
function stableEventKey(event: SessionEvent): string {
  const planKey = planSurfaceKey(event)
  if (planKey) return planKey
  if (event.type === 'acp' && event.acp?.id) {
    if (event.acp.tool_calls?.length) return `acp_tools:${event.acp.id}`
    if (event.acp.error) return `acp_error:${event.acp.id}`
    return `acp_status:${event.acp.id}`
  }
  if (event.type === 'acp_tool' && event.acp?.id && event.acp.tool_calls?.[0]?.id) {
    return `acp_tool:${event.acp.id}:${event.acp.tool_calls[0].id}`
  }
  if ((event.type === 'permission_request' || event.type === 'permission_response') && event.permission?.id) {
    return `${event.type}:${event.permission.id}`
  }
  return `${event.session_id}:${event.seq ?? 'live'}`
}

function combineSequentialACPText(events: SessionEvent[]): SessionEvent[] {
  const out: SessionEvent[] = []
  let lastSourceSeq = 0
  for (const event of events) {
    const prev = out.at(-1)
    // A seq gap means another event sat between these chunks; keep the boundary.
    const contiguous = !prev?.seq || !event.seq || event.seq === lastSourceSeq + 1
    lastSourceSeq = event.seq ?? 0
    if (canMergeACPText(prev, event) && contiguous) {
      const merged: SessionEvent = {
        ...prev!,
        content:
          event.type === 'acp_message'
            ? `${prev!.content ?? ''}${event.content ?? ''}`
            : prev!.content,
        acp: prev!.acp
          ? {
              ...prev!.acp,
              thought:
                event.type === 'acp_thought'
                  ? `${prev!.acp.thought ?? ''}${event.acp?.thought ?? ''}`
                  : prev!.acp.thought,
              state: event.acp?.state ?? prev!.acp.state,
              stop_reason: event.acp?.stop_reason ?? prev!.acp.stop_reason,
              error: event.acp?.error ?? prev!.acp.error,
            }
          : prev!.acp,
      }
      out[out.length - 1] = merged
      continue
    }
    out.push(event)
  }
  return out
}

function canMergeACPText(prev: SessionEvent | undefined, event: SessionEvent): boolean {
  if (!prev?.acp || !event.acp) return false
  if (prev.type !== event.type) return false
  if (event.type !== 'acp_message' && event.type !== 'acp_thought') return false
  return prev.acp.id === event.acp.id
}
