import { motion } from 'motion/react'
import { useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import {
  Check,
  CheckCircle2,
  ChevronRight,
  Circle,
  LoaderCircle,
  Maximize2,
  Minimize2,
  X,
} from 'lucide-react'
import { useEffect, useRef, useState, type ReactNode } from 'react'
import { answerSessionInteractiveResponse } from '@/lib/api/sessions'
import type {
  ACPPermission,
  ACPPlanEntry,
  ACPToolCall,
  ChatMessage,
  SessionEvent,
} from '@/lib/api/types'
import { relativeTime } from '@/lib/format/time'
import { keys } from '@/lib/query/keys'
import { MessageMarkdown } from './MessageMarkdown'
import { ThinkingBlock } from './ThinkingBlock'
import { ToolCallCard } from './ToolCallCard'

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

function Bubble({ message }: { message: ChatMessage }) {
  switch (message.role) {
    case 'user':
      return (
        <div className="flex justify-end">
          <div className="max-w-[80%] rounded-card bg-surface px-3.5 py-2.5 text-sm whitespace-pre-wrap select-text">
            {messageText(message)}
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
            ?.filter((block) => block.type === 'tool')
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
}

function normalized(value: string | undefined): string {
  return (value ?? '').trim().toLowerCase()
}

function hasPlanSurface(event: SessionEvent): boolean {
  return Boolean(event.acp?.plan?.length)
}

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
    !hasPlanSurface(event) &&
    !event.content &&
    !event.acp?.thought &&
    !event.acp?.error &&
    !event.acp?.tool_calls?.length
  )
}

function hasPermissionSurface(permission: ACPPermission | undefined): boolean {
  if (!permission?.id?.trim()) return false
  return Boolean(
    permission.questions?.length ||
      permission.options?.length ||
      permission.locations?.length,
  )
}

function hasVisibleACPSurface(event: SessionEvent): boolean {
  const acp = event.acp
  if (!acp) return false
  if (isParentChildACPEvent(event)) {
    return Boolean(
      event.content ||
        acp.thought ||
        acp.error ||
        hasPlanSurface(event) ||
        hasWorkingStatusSurface(event),
    )
  }
  return Boolean(
    event.content ||
      acp.thought ||
      acp.error ||
      acp.tool_calls?.length ||
      hasPlanSurface(event) ||
      hasWorkingStatusSurface(event),
  )
}

function agentLabel(value: string | undefined): string {
  const normalizedAgent = (value || 'agent').trim()
  if (!normalizedAgent) return 'Agent'
  if (normalizedAgent.toLowerCase() === 'codex') return 'Codex'
  return normalizedAgent
    .split(/[_\s-]+/)
    .filter(Boolean)
    .map((part) => part.slice(0, 1).toUpperCase() + part.slice(1))
    .join(' ')
}

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

function ToolDisclosure({ label, calls, active = false }: { label: string; calls: ACPToolCall[]; active?: boolean }) {
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
        className="inline-flex min-h-7 items-center gap-1.5 rounded-control px-1 text-left font-mono text-[12px] text-ink-3 transition-colors hover:text-ink"
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
}

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

function PlanStatusIcon({ status }: { status?: string }) {
  switch (normalized(status)) {
    case 'completed':
    case 'complete':
      return <CheckCircle2 className="mt-0.5 size-3.5 text-ok" aria-hidden />
    case 'in_progress':
    case 'in-progress':
      return <LoaderCircle className="mt-0.5 size-3.5 animate-spin text-running" aria-hidden />
    default:
      return <Circle className="mt-0.5 size-3.5 text-ink-3" aria-hidden />
  }
}

function PlanChecklist({
  entries,
  onApprovePlan,
}: {
  entries?: ACPPlanEntry[]
  onApprovePlan?: () => void
}) {
  const [expanded, setExpanded] = useState(false)
  const [overflowing, setOverflowing] = useState(false)
  const contentRef = useRef<HTMLDivElement>(null)

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
  }, [entries, expanded])

  const showExpandControl = expanded || overflowing

  return (
    <div className="rounded-card border border-border bg-surface/60 px-3 py-2.5">
      <div className="mb-2 flex items-center justify-between gap-3">
        <p className="text-[11px] font-medium tracking-wide text-ink-3 uppercase">Plan</p>
        <div className="flex shrink-0 items-center gap-1.5">
          {showExpandControl ? (
            <button
              type="button"
              onClick={() => setExpanded((value) => !value)}
              className="inline-flex h-7 items-center gap-1.5 rounded-control border border-border bg-bg px-2 text-[12px] font-medium text-ink-2 transition-colors hover:border-primary hover:text-primary"
              aria-label={expanded ? 'Collapse plan' : 'Expand plan'}
              title={expanded ? 'Collapse plan' : 'Expand plan'}
            >
              {expanded ? <Minimize2 size={13} /> : <Maximize2 size={13} />}
              {expanded ? 'Collapse' : 'Expand'}
            </button>
          ) : null}
          {onApprovePlan ? (
            <button
              type="button"
              onClick={onApprovePlan}
              className="inline-flex h-7 items-center gap-1.5 rounded-control bg-primary px-2 text-[12px] font-medium text-white transition-colors hover:bg-primary-strong"
            >
              <Check size={13} />
              Approve plan
            </button>
          ) : null}
        </div>
      </div>
      <div
        ref={contentRef}
        className={`relative ${expanded ? '' : 'max-h-[340px] overflow-hidden'}`}
      >
        <ul className="flex flex-col gap-3 pb-1">
          {entries?.map((entry, index) => (
            <li
              key={`${entry.content}-${index}`}
              className="flex min-w-0 gap-2 text-sm text-ink-2"
            >
              <PlanStatusIcon status={entry.status} />
              <div className="min-w-0 flex-1">
                <MessageMarkdown text={entry.content} />
              </div>
            </li>
          ))}
        </ul>
        {!expanded && overflowing ? (
          <div
            className="pointer-events-none absolute inset-x-0 bottom-0 h-20 bg-gradient-to-b from-transparent via-surface/85 to-surface"
            aria-hidden
          />
        ) : null}
      </div>
    </div>
  )
}

function QuestionPermissionCard({
  event,
  resolution,
}: {
  event: SessionEvent
  resolution?: ACPPermission
}) {
  const permission = event.permission
  const queryClient = useQueryClient()
  const [answers, setAnswers] = useState<Record<string, string>>({})
  const [submitting, setSubmitting] = useState(false)
  const [localAnswered, setLocalAnswered] = useState(false)
  const [open, setOpen] = useState(false)
  const [error, setError] = useState('')
  if (!permission?.questions?.length) return null
  const questions = permission.questions

  const status = normalized(resolution?.status || permission.status)
  const answered = localAnswered || status === 'selected' || Boolean(resolution?.selected_option_id)
  const cancelled = status === 'cancelled'
  const locked = answered || cancelled || submitting
  const complete = questions.every((question) => answers[question.id]?.trim())

  // Settled questions collapse to a single line, codex-style.
  if ((answered || cancelled) && !open) {
    return (
      <button
        type="button"
        aria-expanded={false}
        onClick={() => setOpen(true)}
        className="inline-flex min-h-7 items-center gap-1.5 self-start rounded-control px-1 text-left font-mono text-[12px] text-ink-3 transition-colors hover:text-ink"
      >
        <ChevronRight size={12} className="shrink-0" aria-hidden />
        Asked {questions.length} question{questions.length === 1 ? '' : 's'}
        {cancelled ? ' · cancelled' : ''}
      </button>
    )
  }

  const setAnswer = (questionID: string, value: string) => {
    setAnswers((current) => ({ ...current, [questionID]: value }))
  }

  const submit = async () => {
    if (!complete || locked) return
    setSubmitting(true)
    setError('')
    try {
      await answerSessionInteractiveResponse(event.session_id, {
        request_id: permission.id,
        answers: Object.fromEntries(
          questions.map((question) => [
            question.id,
            { answers: [answers[question.id].trim()] },
          ]),
        ),
      })
      setLocalAnswered(true)
      await queryClient.invalidateQueries({ queryKey: keys.sessionMessages(event.session_id) })
      queryClient.invalidateQueries({ queryKey: keys.sidebarSessions })
      queryClient.invalidateQueries({ queryKey: keys.allSessions })
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Question response failed.')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="rounded-card border border-border bg-surface px-3 py-2.5">
      <div className="flex items-start justify-between gap-3">
        <p className="text-sm font-medium text-ink">{permission.title || 'Clarifying questions'}</p>
        {answered ? (
          <span className="inline-flex shrink-0 items-center gap-1 text-[12px] text-ok">
            <Check className="size-3.5" aria-hidden />
            Answered
          </span>
        ) : cancelled ? (
          <span className="inline-flex shrink-0 items-center gap-1 text-[12px] text-danger">
            <X className="size-3.5" aria-hidden />
            Cancelled
          </span>
        ) : null}
      </div>

      <div className="mt-3 flex flex-col gap-3">
        {questions.map((question, index) => {
          const selected = answers[question.id] ?? ''
          const options = question.options ?? []
          return (
            <div key={question.id} className="rounded-control bg-bg px-2.5 py-2">
              <div className="flex gap-2">
                <span className="mt-0.5 font-mono text-[11px] text-ink-3">{index + 1}</span>
                <div className="min-w-0 flex-1">
                  {question.header ? (
                    <p className="text-[11px] font-medium tracking-wide text-ink-3 uppercase">
                      {question.header}
                    </p>
                  ) : null}
                  <p className="text-sm text-ink">{question.question}</p>
                  {options.length ? (
                    <div className="mt-2 flex flex-wrap gap-1.5">
                      {options.map((option) => (
                        <button
                          key={option.label}
                          type="button"
                          disabled={locked}
                          onClick={() => setAnswer(question.id, option.label)}
                          className={`inline-flex min-h-8 items-center rounded-control border px-2 py-1 text-left text-[12px] font-medium transition-colors disabled:cursor-not-allowed disabled:opacity-60 ${
                            selected === option.label
                              ? 'border-primary bg-primary-soft text-primary-strong'
                              : 'border-border bg-surface text-ink hover:border-primary hover:text-primary'
                          }`}
                          title={option.description || option.label}
                        >
                          {option.label}
                        </button>
                      ))}
                    </div>
                  ) : null}
                  {question.is_other || !options.length ? (
                    <input
                      type={question.is_secret ? 'password' : 'text'}
                      value={!options.length || !options.some((option) => option.label === selected) ? selected : ''}
                      disabled={locked}
                      placeholder={options.length ? 'Other answer...' : 'Answer...'}
                      className="mt-2 h-8 w-full rounded-control border border-border bg-surface px-2 text-[12px] text-ink placeholder:text-ink-3 disabled:cursor-not-allowed disabled:opacity-60"
                      onChange={(e) => setAnswer(question.id, e.target.value)}
                    />
                  ) : null}
                </div>
              </div>
            </div>
          )
        })}
      </div>

      {!answered && !cancelled ? (
        <div className="mt-3 flex justify-end">
          <button
            type="button"
            disabled={!complete || locked}
            onClick={() => void submit()}
            className="inline-flex h-8 items-center gap-1.5 rounded-control bg-primary px-3 text-[12px] font-medium text-white transition hover:bg-primary-strong disabled:cursor-not-allowed disabled:bg-bg disabled:text-ink-3"
          >
            {submitting ? (
              <LoaderCircle className="size-3.5 animate-spin" aria-hidden />
            ) : (
              <Check className="size-3.5" aria-hidden />
            )}
            Submit answers
          </button>
        </div>
      ) : null}
      {error ? <p className="mt-2 text-[12px] text-danger">{error}</p> : null}
    </div>
  )
}

function PermissionCard({
  event,
  resolution,
}: {
  event: SessionEvent
  resolution?: ACPPermission
}) {
  const permission = event.permission
  const [localSelection, setLocalSelection] = useState('')
  const [submitting, setSubmitting] = useState('')
  const [text, setText] = useState('')
  const [error, setError] = useState('')
  if (!permission) return null
  if (!hasPermissionSurface(permission)) return null
  if (permission.questions?.length) {
    return <QuestionPermissionCard event={event} resolution={resolution} />
  }

  const selected = localSelection || resolution?.selected_option_id || permission.selected_option_id || ''
  const status = normalized(resolution?.status || permission.status)
  const cancelled = status === 'cancelled'
  const locked = Boolean(selected) || cancelled || Boolean(submitting)

  const choose = async (optionID: string) => {
    setSubmitting(optionID)
    setError('')
    try {
      await answerSessionInteractiveResponse(event.session_id, {
        request_id: permission.id,
        option_id: optionID,
      })
      setLocalSelection(optionID)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Permission response failed.')
    } finally {
      setSubmitting('')
    }
  }

  const sendText = async () => {
    const trimmed = text.trim()
    if (!trimmed || locked) return
    setSubmitting('text')
    setError('')
    try {
      await answerSessionInteractiveResponse(event.session_id, {
        request_id: permission.id,
        text: trimmed,
      })
      setText('')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Permission response failed.')
    } finally {
      setSubmitting('')
    }
  }

  return (
    <div className="rounded-card border border-border bg-surface px-3 py-2.5">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="text-sm font-medium text-ink">{permission.title || 'Permission requested'}</p>
          {permission.locations?.length ? (
            <div className="mt-1 flex flex-wrap gap-1.5">
              {permission.locations.map((location) => (
                <span
                  key={`${location.path}:${location.line ?? 0}`}
                  className="rounded border border-border bg-bg px-1.5 py-px font-mono text-[11px] text-ink-2"
                >
                  {location.path}
                  {location.line ? `:${location.line}` : ''}
                </span>
              ))}
            </div>
          ) : null}
        </div>
        {selected ? (
          <span className="inline-flex shrink-0 items-center gap-1 text-[12px] text-ok">
            <Check className="size-3.5" aria-hidden />
            {permission.options?.find((option) => option.id === selected)?.name || selected}
          </span>
        ) : cancelled ? (
          <span className="inline-flex shrink-0 items-center gap-1 text-[12px] text-danger">
            <X className="size-3.5" aria-hidden />
            Cancelled
          </span>
        ) : null}
      </div>

      {!selected && !cancelled && permission.options?.length ? (
        <div className="mt-2 flex flex-wrap gap-1.5">
          {permission.options.map((option) => (
            <button
              key={option.id}
              type="button"
              disabled={locked}
              onClick={() => void choose(option.id)}
              className="inline-flex h-7 items-center gap-1.5 rounded-control border border-border bg-bg px-2 text-[12px] font-medium text-ink transition hover:border-primary hover:text-primary disabled:cursor-not-allowed disabled:opacity-60"
            >
              {submitting === option.id ? (
                <LoaderCircle className="size-3.5 animate-spin" aria-hidden />
              ) : (
                <Check className="size-3.5" aria-hidden />
              )}
              {option.name}
            </button>
          ))}
        </div>
      ) : null}
      {!selected && !cancelled ? (
        <div className="mt-2 flex items-end gap-1.5">
          <textarea
            value={text}
            rows={1}
            disabled={locked}
            placeholder="Reply with details..."
            className="min-h-8 flex-1 resize-none rounded-control border border-border bg-bg px-2 py-1.5 text-[12px] text-ink placeholder:text-ink-3 disabled:cursor-not-allowed disabled:opacity-60"
            onChange={(e) => setText(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter' && !e.shiftKey) {
                e.preventDefault()
                void sendText()
              }
            }}
          />
          <button
            type="button"
            disabled={!text.trim() || locked}
            onClick={() => void sendText()}
            className="inline-flex h-8 items-center gap-1.5 rounded-control border border-border bg-bg px-2 text-[12px] font-medium text-ink transition hover:border-primary hover:text-primary disabled:cursor-not-allowed disabled:opacity-60"
          >
            {submitting === 'text' ? (
              <LoaderCircle className="size-3.5 animate-spin" aria-hidden />
            ) : (
              <Check className="size-3.5" aria-hidden />
            )}
            Reply
          </button>
        </div>
      ) : null}
      {error ? <p className="mt-2 text-[12px] text-danger">{error}</p> : null}
    </div>
  )
}

function LiveEvent({
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
  const canApprovePlan = Boolean(
    event.acp?.modes?.plan_mode_id &&
      event.acp.modes.current_mode_id === event.acp.modes.plan_mode_id &&
      onApprovePlan,
  )
  const ownSession = Boolean(event.acp && event.acp.id === sessionId)
  const showWorkingStatus =
    event.type === 'acp' && hasWorkingStatusSurface(event) && !hasPlanSurface(event) && !ownSession
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
          {event.acp.title ? ` · ${event.acp.title}` : ''} · {relativeTime(event.at)}
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
          <span>{agentLabel(event.acp?.agent)} is working on {childLabel(event)}</span>
        </Link>
      ) : null}
      {!parentChild ? <ToolSummary calls={event.acp?.tool_calls} active={working} /> : null}
      {event.permission ? (
        <PermissionCard event={event} resolution={permissionResolution} />
      ) : null}
      {showPlan && event.acp?.plan?.length ? (
        <PlanChecklist
          entries={event.acp.plan}
          onApprovePlan={canApprovePlan ? onApprovePlan : undefined}
        />
      ) : null}
    </motion.div>
  )
}

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
  if (event.type === 'acp') {
    if (event.acp?.error) return false
    if (hasPlanSurface(event) && latestPlanIndex.get(event.acp!.id) === item.eventIndex) return false
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
        className="inline-flex min-h-7 items-center gap-1.5 self-start rounded-control px-1 text-left text-[12px] font-medium text-ink-3 transition-colors hover:text-ink"
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

export function Transcript({
  messages,
  events,
  sessionId,
  groupTurns = false,
  working = false,
  tail,
  onApprovePlan,
}: {
  messages: ChatMessage[]
  events: SessionEvent[]
  sessionId?: string
  groupTurns?: boolean
  working?: boolean
  // in-flight live exchange, rendered between history and anchored live state
  tail?: ReactNode
  onApprovePlan?: () => void
}) {
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
    if (acp && hasPlanSurface(event)) {
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
      if (!acp) return true
      if (!hasVisibleACPSurface(event)) return false
      // This page's own running state has no link to render — drop the event
      // instead of leaving an empty row.
      if (isWorkingLinkOnly(event) && acp.id === sessionId) return false
      if (hasPlanSurface(event)) {
        const isLatestPlan = latestPlanEvent.get(acp.id) === index
        if (!isLatestPlan && !event.content && !acp.error && !acp.tool_calls?.length) return false
      }
      if (event.type === 'acp' && acp.tool_calls?.length && latestToolEvent.get(acp.id) !== index) {
        return Boolean(event.content || acp.error || hasPlanSurface(event))
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

  const visibleMessages = messages.filter(
    (message) => message.role === 'user' || message.role === 'assistant',
  )
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
      case 'event':
        return (
          <LiveEvent
            key={`event-${stableEventKey(item.event)}`}
            event={item.event}
            sessionId={sessionId}
            showHeader={item.showHeader}
            working={working}
            showPlan={
              hasPlanSurface(item.event)
                ? latestPlanEvent.get(item.event.acp!.id) === item.eventIndex
                : false
            }
            onApprovePlan={onApprovePlan}
            permissionResolution={
              item.event.permission ? permissionResolutions.get(item.event.permission.id) : undefined
            }
          />
        )
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

  const turns = splitTurns(chronological)
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
}

// Coalesced events keep their latest copy whose seq changes per update; key by
// identity so streamed deltas patch in place instead of remounting.
function stableEventKey(event: SessionEvent): string {
  if (event.type === 'acp' && event.acp?.id) {
    if (event.acp.plan?.length) return `acp_plan:${event.acp.id}`
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
