import { Link } from '@tanstack/react-router'
import {
  Ban,
  CheckCircle2,
  ChevronDown,
  ChevronRight,
  CircleAlert,
  LoaderCircle,
  type LucideIcon,
} from 'lucide-react'
import { AnimatePresence, motion, useReducedMotion } from 'motion/react'
import { useState, type ReactNode } from 'react'
import { agentLabel } from '@/lib/agentLabel'
import { looksLikeOpaqueToolID, type ProviderSubagentView } from '@/lib/providerSubagents'
import type { SpawnedThreadView } from '@/lib/spawnedThreads'

const AUTO_EXPAND_LIMIT = 12

type SubagentStatus = 'working' | 'completed' | 'failed' | 'cancelled'
type ThreadStatus = 'running' | 'idle' | 'failed' | 'cancelled'
type RunStatus = { label: string; className: string; Icon: LucideIcon; spin?: boolean }

const SUBAGENT_STATUS: Record<SubagentStatus, RunStatus> = {
  working: { label: 'working', className: 'text-running', Icon: LoaderCircle, spin: true },
  completed: { label: 'completed', className: 'text-primary', Icon: CheckCircle2 },
  failed: { label: 'failed', className: 'text-danger', Icon: CircleAlert },
  cancelled: { label: 'cancelled', className: 'text-ink-3', Icon: Ban },
}

const THREAD_STATUS: Record<ThreadStatus, RunStatus> = {
  running: { label: 'running', className: 'text-running', Icon: LoaderCircle, spin: true },
  idle: { label: 'idle', className: 'text-primary', Icon: CheckCircle2 },
  failed: { label: 'failed', className: 'text-danger', Icon: CircleAlert },
  cancelled: { label: 'cancelled', className: 'text-ink-3', Icon: Ban },
}

export function OverviewRuns({
  threads,
  subagents,
}: {
  threads: SpawnedThreadView[]
  subagents: ProviderSubagentView[]
}) {
  return (
    <>
      {threads.length ? <ThreadsSection threads={threads} /> : null}
      {subagents.length ? <SubagentsSection subagents={subagents} /> : null}
    </>
  )
}

export function SectionHeader({ children }: { children: ReactNode }) {
  return <p className="text-[11px] font-medium tracking-wide text-ink-3 uppercase">{children}</p>
}

export function OverviewDisclosureSection({
  title,
  meta,
  open,
  onToggle,
  children,
}: {
  title: ReactNode
  meta?: ReactNode
  open: boolean
  onToggle: () => void
  children: ReactNode
}) {
  const reduceMotion = useReducedMotion()
  return (
    <section>
      <button
        type="button"
        aria-expanded={open}
        onClick={onToggle}
        className="flex w-full cursor-pointer items-center justify-between gap-2 rounded-md text-left transition-colors duration-150 hover:text-ink"
      >
        <SectionHeader>
          {title}
          {meta ? <span className="ml-2 font-mono tabular-nums normal-case tracking-normal">{meta}</span> : null}
        </SectionHeader>
        <ChevronDown
          size={13}
          className={`shrink-0 text-ink-3 transition-transform duration-200 ease-out ${open ? '' : '-rotate-90'}`}
          aria-hidden
        />
      </button>
      <AnimatePresence initial={false}>
        {open ? (
          <motion.div
            initial={reduceMotion ? { opacity: 0 } : { height: 0, opacity: 0, y: -2 }}
            animate={reduceMotion ? { opacity: 1 } : { height: 'auto', opacity: 1, y: 0 }}
            exit={reduceMotion ? { opacity: 0 } : { height: 0, opacity: 0, y: -2 }}
            transition={reduceMotion ? { duration: 0.12 } : { duration: 0.18, ease: 'easeOut' }}
            className="overflow-hidden"
          >
            {children}
          </motion.div>
        ) : null}
      </AnimatePresence>
    </section>
  )
}

function ThreadsSection({ threads }: { threads: SpawnedThreadView[] }) {
  const [open, setOpen] = useState(threads.length <= AUTO_EXPAND_LIMIT)
  return (
    <OverviewDisclosureSection
      title="Threads"
      meta={threads.length}
      open={open}
      onToggle={() => setOpen((value) => !value)}
    >
      <ul className="mt-2 flex flex-col">
        {threads.map((thread) => (
          <ThreadRow key={thread.key} thread={thread} />
        ))}
      </ul>
    </OverviewDisclosureSection>
  )
}

function ThreadRow({ thread }: { thread: SpawnedThreadView }) {
  const status = THREAD_STATUS[threadStatus(thread.state)]
  const title = firstText(thread.title, thread.slug) || 'Thread'
  const detail = threadDetail(thread)
  return (
    <li className="min-w-0 rounded-md">
      <Link
        to="/sessions/$sessionId"
        params={{ sessionId: thread.id }}
        title={`Open ${title}`}
        className="flex h-8 w-full min-w-0 items-center gap-2 rounded-md px-2 text-left transition-colors duration-150 hover:bg-surface-2"
      >
        <RunRowContent
          title={title}
          detail={detail}
          status={status}
          trailing={<ChevronRight size={13} className="shrink-0 text-ink-3" aria-hidden />}
        />
      </Link>
    </li>
  )
}

function threadStatus(state: string | undefined): ThreadStatus {
  const normalized = state?.toLowerCase()
  if (normalized === 'idle' || normalized === 'completed') return 'idle'
  if (normalized === 'failed' || normalized === 'errored' || normalized === 'error') return 'failed'
  if (normalized === 'cancelled' || normalized === 'canceled' || normalized === 'interrupted') return 'cancelled'
  return 'running'
}

function threadDetail(thread: SpawnedThreadView): string {
  const model = compactModel(thread.model)
  return [
    agentLabel(thread.agent),
    model ? withReasoningEffort(model, thread.reasoning_effort) : '',
    thread.archived ? 'Archived' : '',
  ]
    .filter(Boolean)
    .join(' · ')
}

function compactModel(model?: string): string {
  if (!model) return ''
  const parts = model.split('/').filter(Boolean)
  return parts.at(-1) ?? model
}

function withReasoningEffort(model: string, effort?: string): string {
  return effort ? `${model}/${effort}` : model
}

// One line: where the run stands, then what it is. Its agent, model or task
// shows on hover.
function RunRowContent({
  title,
  detail,
  status,
  trailing,
}: {
  title: string
  detail?: string
  status: RunStatus
  trailing?: ReactNode
}) {
  return (
    <>
      <span
        className={`inline-flex size-5 shrink-0 items-center justify-center ${status.className}`}
        title={status.label}
        aria-label={status.label}
      >
        <status.Icon size={14} className={status.spin ? 'animate-spin' : ''} aria-hidden />
      </span>
      <span className="min-w-0 flex-1 truncate text-[13px] text-ink" title={detail ? `${title}\n${detail}` : title}>
        {title}
      </span>
      {trailing}
    </>
  )
}

function SubagentsSection({ subagents }: { subagents: ProviderSubagentView[] }) {
  const [open, setOpen] = useState(subagents.length <= AUTO_EXPAND_LIMIT)
  return (
    <OverviewDisclosureSection
      title="Subagents"
      meta={subagents.length}
      open={open}
      onToggle={() => setOpen((value) => !value)}
    >
      <ul className="mt-2 flex flex-col">
        {subagents.map((subagent) => (
          <SubagentRow key={subagent.key} subagent={subagent} />
        ))}
      </ul>
    </OverviewDisclosureSection>
  )
}

function SubagentRow({ subagent }: { subagent: ProviderSubagentView }) {
  const [expanded, setExpanded] = useState(false)
  const status = SUBAGENT_STATUS[subagentStatus(subagent.status)]
  const title = subagentTitle(subagent)
  const detail = subagentDetail(subagent, title)
  const prompt = subagent.prompt?.trim() ?? ''
  return (
    <li className="min-w-0 rounded-md">
      <button
        type="button"
        disabled={!prompt}
        title={prompt ? (expanded ? 'Hide prompt' : 'Show prompt') : undefined}
        onClick={() => setExpanded((open) => !open)}
        className="flex h-8 w-full min-w-0 items-center gap-2 rounded-md px-2 text-left transition-colors duration-150 enabled:cursor-pointer enabled:hover:bg-surface-2 disabled:cursor-default"
      >
        <RunRowContent
          title={title}
          detail={detail && detail !== title ? detail : undefined}
          status={status}
          trailing={
            prompt ? (
              <ChevronDown
                size={13}
                className={`shrink-0 text-ink-3 transition-transform duration-150 ${expanded ? 'rotate-180' : ''}`}
                aria-hidden
              />
            ) : null
          }
        />
      </button>
      {expanded && prompt ? (
        <p className="ml-7 max-h-28 overflow-y-auto whitespace-pre-wrap px-1 pb-1 text-[11px] leading-snug text-ink-3">
          {prompt}
        </p>
      ) : null}
    </li>
  )
}

function subagentStatus(status: string | undefined): SubagentStatus {
  const normalized = status?.toLowerCase()
  if (normalized === 'completed' || normalized === 'shutdown' || normalized === 'closed') return 'completed'
  if (normalized === 'failed' || normalized === 'errored' || normalized === 'error' || normalized === 'not_found') {
    return 'failed'
  }
  if (normalized === 'cancelled' || normalized === 'canceled' || normalized === 'interrupted') return 'cancelled'
  return 'working'
}

function subagentTitle(subagent: ProviderSubagentView): string {
  const name = subagent.name?.trim()
  const task = subagent.task?.trim()
  if (name && task && name.toLowerCase() === task.toLowerCase()) return humanizeSubagentTask(task)
  const displayName = name && /[_-]/.test(name) ? humanizeSubagentTask(name) : name
  return firstText(displayName, task ? humanizeSubagentTask(task) : undefined, subagent.role) || 'Subagent'
}

function subagentDetail(subagent: ProviderSubagentView, title: string): string | undefined {
  const task = subagent.task?.trim()
  const taskLabel = task ? humanizeSubagentTask(task) : undefined
  return firstText(
    taskLabel && taskLabel !== title ? taskLabel : undefined,
    subagentSummary(subagent.summary),
    subagent.role?.trim() !== title ? subagent.role : undefined,
  ) || undefined
}

function humanizeSubagentTask(task: string): string {
  const text = task.replace(/[_-]+/g, ' ').replace(/\s+/g, ' ').trim()
  return text ? text[0].toUpperCase() + text.slice(1) : task
}

function subagentSummary(summary: string | undefined): string | undefined {
  const text = summary?.trim()
  if (!text || looksLikeOpaqueToolID(text)) return undefined
  switch (text.toLowerCase()) {
    case 'spawned':
    case 'working':
    case 'waiting':
    case 'wait finished':
    case 'responded':
      return undefined
    default:
      return text
  }
}

function firstText(...values: Array<string | undefined>): string {
  return values.find((value) => value?.trim())?.trim() ?? ''
}
