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
  type PlanStepState,
  type PlanSurface,
} from '@/lib/planSurface'
import { MentionText } from './mentions'
import { MessageMarkdown } from './MessageMarkdown'
import { ThinkingBlock } from './ThinkingBlock'
import { PermissionCard } from './TranscriptPermissions'
import { normalized } from './TranscriptUtils'
import {
  buildTimeline,
  hasWorkingStatusSurface,
  isCollapsibleWork,
  isParentChildACPEvent,
  stableEventKey,
  type TimelineItem,
} from './timeline'
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
            <MentionText text={messageText(message)} />
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

function childLabel(event: SessionEvent): string {
  const acp = event.acp
  return acp?.title || acp?.slug || 'child task'
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
  showHeader,
  working = false,
  permissionResolution,
  showPlan,
  onApprovePlan,
}: {
  event: SessionEvent
  sessionId?: string
  showHeader: boolean
  working?: boolean
  permissionResolution?: ACPPermission
  showPlan?: boolean
  onApprovePlan?: () => void
}) {
  const eventPlan = planSurfaceFromEvent(event)
  const planSurface = showPlan ? eventPlan : undefined
  const ownSession = Boolean(event.acp && event.acp.id === sessionId)
  const showWorkingStatus =
    event.type === 'acp' && hasWorkingStatusSurface(event) && !eventPlan && !ownSession
  const parentChild = isParentChildACPEvent(event)
  return (
    <div className="flex max-w-[72ch] flex-col gap-2">
      {event.acp && showHeader ? (
        <p className="text-[12px] text-ink-3">
          <span className="font-mono">{event.acp.agent}</span>
          {event.acp.title ? ` · ${event.acp.title}` : ''} · {relativeTime(event.at)}
        </p>
      ) : null}
      {event.acp?.thought ? <ThinkingBlock text={event.acp.thought} /> : null}
      {event.content ? <MessageMarkdown text={event.content} /> : null}
      {showWorkingStatus ? (
        <Link
          to="/sessions/$sessionId"
          params={{ sessionId: event.acp?.id ?? event.session_id }}
          className="inline-flex w-fit items-center gap-2 rounded-card border border-border bg-surface px-3 py-2 text-sm text-ink-2 transition-colors hover:border-primary hover:text-primary"
        >
          <LoaderCircle className="size-4 animate-spin text-running" aria-hidden />
          <span>{agentLabel(event.acp?.agent)} is working on {childLabel(event)}</span>
        </Link>
      ) : null}
      {!parentChild ? <ToolSummary calls={event.acp?.tool_calls} active={working} /> : null}
      {event.permission ? (
        <PermissionCard event={event} resolution={permissionResolution} />
      ) : null}
      {planSurface ? (
        <PlanChecklist surface={planSurface} active={working} onApprovePlan={onApprovePlan} />
      ) : null}
    </div>
  )
})

function formatDuration(ms: number): string {
  const totalSeconds = Math.max(1, Math.round(ms / 1000))
  const hours = Math.floor(totalSeconds / 3600)
  const minutes = Math.floor((totalSeconds % 3600) / 60)
  const seconds = totalSeconds % 60
  if (hours) return `${hours}h ${minutes}m`
  if (minutes) return `${minutes}m ${seconds}s`
  return `${seconds}s`
}

function WorkSection({
  items,
  durationMs,
  defaultOpen,
  findActive = false,
  render,
}: {
  items: TimelineItem[]
  durationMs: number
  defaultOpen: boolean
  findActive?: boolean
  render: (item: TimelineItem) => ReactNode
}) {
  const [open, setOpen] = useState(defaultOpen)
  const effectiveOpen = open || findActive

  return (
    <div className="flex flex-col gap-5">
      <button
        type="button"
        aria-expanded={effectiveOpen}
        onClick={() => setOpen((value) => !value)}
        className="inline-flex min-h-7 items-center gap-1.5 self-start rounded-full px-1 text-left text-[12px] font-medium text-ink-3 transition-colors hover:text-ink"
      >
        <ChevronRight
          size={12}
          className={`shrink-0 transition-transform ${effectiveOpen ? 'rotate-90' : ''}`}
          aria-hidden
        />
        Worked for {formatDuration(durationMs)}
      </button>
      {effectiveOpen ? (
        <div className="flex flex-col gap-5 border-l border-border pl-4">
          {items.map((item) => render(item))}
        </div>
      ) : null}
    </div>
  )
}

export const Transcript = memo(function Transcript({
  messages,
  events,
  sessionId,
  groupTurns = false,
  working = false,
  findActive = false,
  tail,
  onApprovePlan,
}: {
  messages: ChatMessage[]
  events: SessionEvent[]
  sessionId?: string
  groupTurns?: boolean
  working?: boolean
  findActive?: boolean
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
              findActive={findActive}
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
