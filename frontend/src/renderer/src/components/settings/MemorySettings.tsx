import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { HelpCircle, RefreshCcw, Sparkles } from 'lucide-react'
import { type ReactNode, useState } from 'react'
import { MarkdownEditor } from '@/components/agent/MarkdownEditor'
import { FileTabs } from './FileTabs'
import { SettingsCard } from './SettingsCard'
import { Button } from '@/components/ui/Button'
import { EmptyState } from '@/components/ui/EmptyState'
import { Select } from '@/components/ui/Select'
import { Skeleton } from '@/components/ui/Skeleton'
import { Switch } from '@/components/ui/Switch'
import { useToast } from '@/components/ui/toast'
import { agentLabel } from '@/lib/agentLabel'
import { agentSettingsQuery } from '@/lib/api/settings'
import {
  dreamMemory,
  memoryQuery,
  reindexMemory,
  saveMemoryHorizon,
  updateMemorySettings,
} from '@/lib/api/memory'
import type { MemoryHorizon, MemoryQueueStatus, MemoryStatus, MemoryTask } from '@/lib/api/types'
import { enabledACPAgents } from '@/lib/agentRuntimes'
import { ReasoningEffortSlider } from '@/components/acp/ReasoningEffortSlider'
import { acpAgentModelSuggestions, modelSuggestionFor, modelSuggestionLabel } from '@/lib/models'
import { modelReasoningEffortOptions } from '@/lib/reasoningEfforts'
import { keys } from '@/lib/query/keys'

const HORIZON_DESCRIPTIONS: Record<string, string> = {
  'LONG_TERM.md':
    'Stable facts agents should remember across sessions: identity, goals, preferences, relationships, and important project context.',
  'SHORT_TERM.md':
    'What is true right now: current focus, active projects, blockers, and open loops.',
}

type TaskCopy = { label: string; description: string }

const TASK_COPY = {
  index_changed_pages: {
    label: 'Update search index',
    description: 'Rebuilds search data for saved memory files.',
  },
  daily_rollup: {
    label: 'Daily memories update',
    description: "Creates today's daily memory file if it does not exist yet.",
  },
  link_hygiene: {
    label: 'Review relationship suggestions',
    description: 'Writes review notes for likely people, company, and project relationships.',
  },
  dream: {
    label: 'Dream',
    description: 'Consolidates durable facts from daily notes into long-term and short-term memory.',
  },
  optimize_index: {
    label: 'Optimize search database',
    description: 'Compacts the existing search database. It does not scan memory files.',
  },
} satisfies Record<string, TaskCopy>

type KnownMemoryTask = MemoryTask & { name: keyof typeof TASK_COPY }

const QUEUE_COPY = {
  projection: {
    label: 'Export Connections data',
    description: 'Converts raw connected-account updates into readable source files.',
  },
  memory: {
    label: 'Ingest Connections data to memory',
    description: 'Asks the selected memory agent to extract durable facts from exported source files.',
  },
}

type StatusTone = 'ok' | 'warn' | 'idle'

function formatRelativeTime(value?: string): string {
  if (!value) return 'never'
  const date = new Date(value)
  if (Number.isNaN(date.getTime()) || date.getTime() <= 0) return 'never'
  const diffMs = date.getTime() - Date.now()
  const absMs = Math.abs(diffMs)
  if (absMs < 60_000) return diffMs >= 0 ? 'in 1m' : '1m ago'
  const units = [
    { suffix: 'd', ms: 86_400_000 },
    { suffix: 'h', ms: 3_600_000 },
    { suffix: 'm', ms: 60_000 },
  ]
  const unit = units.find(({ ms }) => absMs >= ms) ?? units[units.length - 1]
  const amount = Math.max(1, Math.round(absMs / unit.ms))
  return diffMs >= 0 ? `in ${amount}${unit.suffix}` : `${amount}${unit.suffix} ago`
}

export function MemorySettings() {
  const status = useQuery({ ...memoryQuery, refetchInterval: 5000 })
  const agentSettings = useQuery(agentSettingsQuery)
  const queryClient = useQueryClient()
  const toast = useToast()

  const [activeHorizon, setActiveHorizon] = useState<string>()
  const [drafts, setDrafts] = useState<Record<string, string>>({})

  const setStatus = (next: MemoryStatus) => queryClient.setQueryData(keys.memory, next)

  const update = useMutation({
    mutationFn: updateMemorySettings,
    onSuccess: setStatus,
    onError: (error: Error) => toast(`Couldn't update memory settings: ${error.message}`, 'danger'),
  })

  const reindex = useMutation({
    mutationFn: reindexMemory,
    onSuccess: (report) => {
      toast(`Refreshed ${report.page_count} pages, ${report.chunk_count} search parts`)
      void queryClient.invalidateQueries({ queryKey: keys.memory })
    },
    onError: (error: Error) => toast(`Search refresh failed: ${error.message}`, 'danger'),
  })

  const runDream = useMutation({
    mutationFn: dreamMemory,
    onSuccess: (result) => {
      toast(result.dream.run_slug ? `Memory review finished: ${result.dream.run_slug}` : 'Memory review finished')
      void queryClient.invalidateQueries({ queryKey: keys.memory })
    },
    onError: (error: Error) => toast(`Memory review failed: ${error.message}`, 'danger'),
  })

  const saveHorizon = useMutation({
    mutationFn: ({ name, content }: { name: string; content: string }) =>
      saveMemoryHorizon(name, content),
    onSuccess: (saved: MemoryHorizon) => {
      queryClient.setQueryData<MemoryStatus>(keys.memory, (prev) =>
        prev
          ? {
              ...prev,
              horizons: prev.horizons.map((h) =>
                h.name === saved.name ? { ...h, content: saved.content, chars: saved.chars } : h,
              ),
            }
          : prev,
      )
      setDrafts((prev) => {
        const next = { ...prev }
        delete next[saved.name]
        return next
      })
      toast(`Saved ${saved.name}`)
    },
    onError: (error: Error, variables) => {
      toast(`Couldn't save ${variables.name}: ${error.message}`, 'danger')
    },
  })

  if (status.isPending) {
    return (
      <div className="mx-auto max-w-[860px] px-10">
        <Skeleton className="mb-4 h-7 w-40" />
        <Skeleton className="mb-4 h-32" />
        <Skeleton className="h-72" />
      </div>
    )
  }

  if (status.isError) {
    return (
      <EmptyState title="Couldn't load memory status">
        <p>{status.error.message}</p>
      </EmptyState>
    )
  }

  const memory = status.data
  const enabled = memory.enabled
  const horizons = memory.horizons
  const activeName = activeHorizon ?? horizons[0]?.name
  const active = horizons.find((h) => h.name === activeName)
  const memoryAgents = enabledACPAgents(agentSettings.data)
  const selectedMemoryAgent = memory.agent ?? ''
  const staleMemoryAgent = selectedMemoryAgent && !memoryAgents.includes(selectedMemoryAgent)
  const memoryAgentOptions = [
    { value: '', label: 'Not selected' },
    ...(staleMemoryAgent
      ? [
          {
            value: selectedMemoryAgent,
            label: `${agentLabel(selectedMemoryAgent)} (disabled)`,
          },
        ]
      : []),
    ...memoryAgents.map((agent) => ({
      value: agent,
      label: agentLabel(agent),
    })),
  ]
  const memoryAgentValid =
    !selectedMemoryAgent || memoryAgents.includes(selectedMemoryAgent) || agentSettings.isPending
  const summary = memoryStatusSummary(memory, selectedMemoryAgent, memoryAgentValid)

  const modelSuggestions = selectedMemoryAgent
    ? acpAgentModelSuggestions(agentSettings.data, selectedMemoryAgent)
    : []
  const defaultModelLabel = memory.default_model
    ? modelSuggestionLabel(modelSuggestions, memory.default_model)
    : 'Agent default'
  const memoryModelOptions = [
    { value: '', label: `${defaultModelLabel} · default` },
    ...modelSuggestions.map((s) => ({ value: s.value, label: s.label })),
  ]
  const effectiveMemoryModel = memory.model || memory.default_model || ''
  const memoryEffortStops = modelReasoningEffortOptions(
    agentSettings.data,
    selectedMemoryAgent,
    effectiveMemoryModel,
    modelSuggestions,
  ).filter((option) => option.value !== '')
  const defaultMemoryEffort =
    memory.default_reasoning_effort ||
    modelSuggestionFor(modelSuggestions, effectiveMemoryModel)?.reasoningDefaultEffort
  const memoryEffort = memory.reasoning_effort ?? ''

  return (
    <div className="flex flex-col gap-5 py-5">
      <header>
        <h1 className="text-lg font-semibold text-ink">Memory</h1>
        <p className="mt-0.5 text-[13px] text-ink-2">Saved context for future agent sessions.</p>
      </header>

      <SettingsCard>
        <MemorySettingsRow
          title="Use memory"
          description={enabled ? 'Included in agent prompts.' : 'Not included in agent prompts.'}
          explanation="This is the main switch. Turning it off stops memory from being added to agent prompts and disables automatic memory work."
        >
          <div className="flex h-8 items-center justify-start gap-2 md:justify-end">
            <span className="text-[12px] text-ink-2">{enabled ? 'Enabled' : 'Disabled'}</span>
            <Switch
              checked={enabled}
              disabled={update.isPending}
              onChange={(next) => update.mutate({ enabled: next })}
              aria-label="Enable memory"
            />
          </div>
        </MemorySettingsRow>

        <MemorySettingsRow
          title="Memory agent"
          description={selectedMemoryAgent ? `${agentLabel(selectedMemoryAgent)} handles upkeep.` : 'Choose an agent.'}
          explanation="This agent answers memory-search requests and does background memory review. It should be an enabled ACP agent you trust with your saved context."
          disabled={!enabled}
        >
          <div className="grid gap-1">
            <Select
              value={selectedMemoryAgent}
              options={memoryAgentOptions}
              disabled={!enabled || update.isPending || agentSettings.isPending}
              onChange={(agent) => update.mutate({ agent })}
              aria-label="Memory agent"
              className="w-full md:w-[260px]"
            />
            {!memoryAgentValid ? (
              <p className="text-[12px] text-danger">{agentLabel(selectedMemoryAgent)} is no longer enabled.</p>
            ) : null}
          </div>
        </MemorySettingsRow>

        {selectedMemoryAgent ? (
          <MemorySettingsRow
            title="Model and reasoning"
            description={
              memory.model || memory.reasoning_effort
                ? 'Custom for memory work.'
                : 'Fast defaults for background work.'
            }
            explanation="Memory upkeep runs many background sessions, so it defaults to a fast, inexpensive model with low reasoning effort. Pick a different model or effort if you want deeper memory work."
            disabled={!enabled}
          >
            <div className="grid w-full gap-2 md:w-[260px]">
              <Select
                value={memory.model ?? ''}
                options={memoryModelOptions}
                disabled={!enabled || update.isPending || agentSettings.isPending}
                onChange={(model) => update.mutate({ model })}
                aria-label="Memory model"
                className="w-full"
              />
              {memoryEffortStops.length > 1 ? (
                <ReasoningEffortSlider
                  options={memoryEffortStops}
                  value={memoryEffort}
                  defaultValue={defaultMemoryEffort}
                  disabled={!enabled || agentSettings.isPending}
                  onChange={(effort) => update.mutate({ reasoning_effort: effort })}
                />
              ) : null}
            </div>
          </MemorySettingsRow>
        ) : null}
      </SettingsCard>

      {enabled ? (
        <div className="flex flex-col gap-5">
          <section>
            <SectionHeader title="Status" />
            <SettingsCard className="mt-3">
              <div className="flex flex-col gap-3 px-4 py-3 md:flex-row md:items-center md:justify-between">
                <div className="min-w-0">
                  <div className="flex flex-wrap items-center gap-2">
                    <p className="text-[13px] font-medium text-ink">{summary.label}</p>
                    <StatusPill tone={summary.tone}>{summary.badge}</StatusPill>
                  </div>
                  {summary.detail ? <p className="mt-0.5 text-[12px] text-ink-2">{summary.detail}</p> : null}
                </div>
                <div className="grid grid-cols-3 gap-2 text-right text-[12px] md:min-w-[280px]">
                  <MiniStat value={memory.doctor.page_count} label="notes" />
                  <MiniStat value={memory.doctor.chunk_count} label="search" />
                  <MiniStat value={memory.doctor.unresolved_count} label="bad links" />
                </div>
              </div>
            </SettingsCard>
          </section>

          <section>
            <SectionHeader title="Background upkeep" />
            <SettingsCard className="mt-3">
              {memory.source_queues ? (
                <div className="grid grid-cols-1 divide-y divide-border border-b border-border md:grid-cols-2 md:divide-x md:divide-y-0">
                  <SourceQueueStatus
                    label={QUEUE_COPY.projection.label}
                    detail={QUEUE_COPY.projection.description}
                    queue={memory.source_queues.projection}
                  />
                  <SourceQueueStatus
                    label={QUEUE_COPY.memory.label}
                    detail={QUEUE_COPY.memory.description}
                    queue={memory.source_queues.memory}
                  />
                </div>
              ) : null}
              <ul className="divide-y divide-border">
                {memory.tasks.filter(isKnownMemoryTask).map((task) => (
                  <MemoryTaskRow key={task.name} task={task} />
                ))}
              </ul>
            </SettingsCard>
          </section>

          <section>
            <SectionHeader title="Manual actions" />
            <SettingsCard className="mt-3">
              <MaintenanceAction
                title="Rebuild search index"
                description="Rebuild search data for saved memory files."
                explanation="Use this if you edited memory files outside Jaz or memory search returns stale results. Normal edits refresh automatically."
              >
                <Button
                  variant="secondary"
                  size="sm"
                  onClick={() => reindex.mutate()}
                  disabled={reindex.isPending || runDream.isPending}
                >
                  <RefreshCcw size={13} />
                  {reindex.isPending ? 'Rebuilding...' : 'Rebuild index'}
                </Button>
              </MaintenanceAction>
              <MaintenanceAction
                title="Review memory now"
                description="Consolidate recent notes."
                explanation="This runs the same review that normally happens in the background. It can use tokens because it asks an agent to read recent memory notes."
              >
                <Button
                  variant="secondary"
                  size="sm"
                  onClick={() => runDream.mutate()}
                  disabled={reindex.isPending || runDream.isPending || !selectedMemoryAgent}
                >
                  <Sparkles size={13} />
                  {runDream.isPending ? 'Reviewing...' : 'Review now'}
                </Button>
              </MaintenanceAction>
            </SettingsCard>
          </section>

          {active ? (
            <section className="flex flex-col">
              <SectionHeader title="Memory files" />
              <div className="mt-3 flex items-end justify-between gap-4 border-b border-border">
                <FileTabs
                  underlineId="memory-horizon-tab-underline"
                  active={active.name}
                  onSelect={setActiveHorizon}
                  tabs={horizons.map((horizon) => {
                    const draft = drafts[horizon.name]
                    return {
                      name: horizon.name,
                      dirty: draft !== undefined && draft !== horizon.content,
                    }
                  })}
                />
                <HorizonSaveControls
                  horizon={active}
                  draft={drafts[active.name]}
                  pending={saveHorizon.isPending}
                  onSave={(content) => saveHorizon.mutate({ name: active.name, content })}
                />
              </div>
              <SettingsCard className="h-64 overflow-hidden">
                <MarkdownEditor
                  key={active.name}
                  initialValue={drafts[active.name] ?? active.content}
                  placeholder={HORIZON_DESCRIPTIONS[active.name] ?? 'Write markdown here.'}
                  onChange={(doc) => setDrafts((prev) => ({ ...prev, [active.name]: doc }))}
                  onSave={() => {
                    if (saveHorizon.isPending) return
                    saveHorizon.mutate({ name: active.name, content: drafts[active.name] ?? active.content })
                  }}
                />
              </SettingsCard>
            </section>
          ) : null}
        </div>
      ) : (
        <SettingsCard className="px-4 py-4">
          <p className="text-[13px] font-medium text-ink">Memory is off</p>
          <p className="mt-0.5 text-[12px] text-ink-2">Agents will not receive saved memory.</p>
        </SettingsCard>
      )}
    </div>
  )
}

function MemorySettingsRow({
  title,
  description,
  explanation,
  disabled,
  children,
}: {
  title: string
  description: string
  explanation: string
  disabled?: boolean
  children: ReactNode
}) {
  return (
    <div
      className={`grid grid-cols-1 gap-2 border-t border-border/70 px-4 py-3 first:border-t-0 md:grid-cols-[minmax(0,1fr)_minmax(220px,320px)] md:items-center ${
        disabled ? 'opacity-50' : ''
      }`}
    >
      <div className="min-w-0">
        <div className="flex items-center gap-1.5">
          <p className="text-[13px] font-medium text-ink">{title}</p>
          <HelpTooltip text={explanation} />
        </div>
        <p className="mt-0.5 text-[12px] text-ink-3">{description}</p>
      </div>
      <div className="min-w-0 md:justify-self-end">{children}</div>
    </div>
  )
}

function SectionHeader({ title, description }: { title: string; description?: string }) {
  return (
    <div>
      <p className="text-sm font-medium text-ink">{title}</p>
      {description ? <p className="mt-0.5 text-[13px] text-ink-2">{description}</p> : null}
    </div>
  )
}

function HelpTooltip({ text }: { text: string }) {
  return (
    <span className="group relative inline-flex">
      <button
        type="button"
        className="grid size-6 place-items-center rounded-full text-ink-3 outline-none transition-colors duration-150 hover:text-ink focus-visible:text-ink"
      >
        <HelpCircle size={12} aria-hidden />
        <span className="sr-only">{text}</span>
      </button>
      <span
        role="tooltip"
        className="pointer-events-none absolute bottom-full left-1/2 z-tooltip mb-1.5 hidden w-[260px] -translate-x-1/2 rounded-[8px] bg-ink px-3 py-2 text-left text-[12px] leading-relaxed text-bg shadow-[0_8px_30px_rgba(0,0,0,0.22)] group-hover:block group-focus-within:block"
      >
        {text}
      </span>
    </span>
  )
}

function SourceQueueStatus({
  label,
  detail,
  queue,
}: {
  label: string
  detail: string
  queue: MemoryQueueStatus
}) {
  const active = queue.pending > 0 || queue.processing > 0
  const status = queue.error ? 'Needs attention' : active ? queueActivity(queue) : 'Idle'
  return (
    <div className="min-w-0 px-4 py-3">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <span className="flex items-center gap-1.5 text-[12px] font-medium text-ink">
            {label}
            <HelpTooltip text={detail} />
          </span>
        </div>
        <StatusPill tone={queue.error ? 'warn' : active ? 'ok' : 'idle'}>{status}</StatusPill>
      </div>
      {queue.error ? (
        <p className="mt-1 truncate text-[11px] text-danger" title={queue.error}>
          {queue.error}
        </p>
      ) : null}
    </div>
  )
}

function MemoryTaskRow({ task }: { task: KnownMemoryTask }) {
  const copy = TASK_COPY[task.name]
  const error = task.status === 'error' || Boolean(task.error)
  return (
    <li className="flex items-center justify-between gap-3 px-4 py-2.5">
      <div className="min-w-0">
        <span className="flex items-center gap-1.5 text-[13px] text-ink">
          {copy.label}
          <HelpTooltip text={copy.description} />
        </span>
        {task.error ? (
          <p className="truncate text-[12px] text-danger" title={task.error}>
            {task.error}
          </p>
        ) : null}
      </div>
      <div className="flex shrink-0 items-center gap-3 text-[12px] text-ink-2">
        <span className={error ? 'font-medium text-danger' : task.last_run_at ? '' : 'text-ink-3'}>
          {error ? 'Needs attention' : task.last_run_at ? `Last ${formatRelativeTime(task.last_run_at)}` : 'Not run yet'}
        </span>
        {task.next_due ? <span className="text-ink-3">Next {formatRelativeTime(task.next_due)}</span> : null}
      </div>
    </li>
  )
}

function isKnownMemoryTask(task: MemoryTask): task is KnownMemoryTask {
  return task.name in TASK_COPY
}

function MaintenanceAction({
  title,
  description,
  explanation,
  children,
}: {
  title: string
  description: string
  explanation: string
  children: ReactNode
}) {
  return (
    <div className="grid grid-cols-1 gap-2 border-t border-border/70 px-4 py-3 first:border-t-0 md:grid-cols-[minmax(0,1fr)_auto] md:items-center">
      <div className="min-w-0">
        <span className="flex items-center gap-1.5 text-[13px] font-medium text-ink">
          {title}
          <HelpTooltip text={explanation} />
        </span>
        <p className="mt-0.5 text-[12px] text-ink-3">{description}</p>
      </div>
      <div className="md:justify-self-end">{children}</div>
    </div>
  )
}

function MiniStat({ value, label }: { value: number; label: string }) {
  return (
    <div className="min-w-0 rounded-[8px] bg-surface-2 px-2 py-1.5">
      <div className="font-mono tabular-nums text-ink">{value}</div>
      <p className="truncate text-[11px] text-ink-3">{label}</p>
    </div>
  )
}

function StatusPill({ tone, children }: { tone: StatusTone; children: ReactNode }) {
  const className =
    tone === 'warn'
      ? 'bg-danger-soft text-danger'
      : tone === 'ok'
        ? 'bg-primary-soft text-primary-strong'
        : 'bg-surface-2 text-ink-3'
  return <span className={`shrink-0 rounded-full px-2 py-0.5 text-[10px] font-medium ${className}`}>{children}</span>
}

function memoryStatusSummary(
  memory: MemoryStatus,
  selectedMemoryAgent: string,
  memoryAgentValid: boolean,
): { label: string; badge: string; detail?: string; tone: StatusTone } {
  if (!selectedMemoryAgent) {
    return {
      label: 'Choose a memory agent',
      badge: 'Needs agent',
      detail: 'Pick an agent to run search and review.',
      tone: 'warn',
    }
  }
  if (!memoryAgentValid) {
    return {
      label: 'Memory agent is unavailable',
      badge: 'Needs agent',
      detail: 'Pick an enabled agent.',
      tone: 'warn',
    }
  }
  if (!memory.scheduler_running) {
    return {
      label: 'Background upkeep is paused',
      badge: 'Paused',
      detail: 'Automatic upkeep is not running.',
      tone: 'warn',
    }
  }
  if (memoryHasErrors(memory)) {
    return {
      label: 'Memory needs attention',
      badge: 'Issue',
      detail: 'A row below has an error.',
      tone: 'warn',
    }
  }
  if (memory.doctor.page_count === 0) {
    return {
      label: 'No saved memory yet',
      badge: 'Empty',
      tone: 'idle',
    }
  }
  return {
    label: 'Memory is ready',
    badge: 'Ready',
    tone: 'ok',
  }
}

function memoryHasErrors(memory: MemoryStatus): boolean {
  return (
    memory.tasks.some((task) => task.status === 'error' || Boolean(task.error)) ||
    Boolean(memory.source_queues?.projection.error || memory.source_queues?.memory.error)
  )
}

function queueActivity(queue: MemoryQueueStatus): string {
  const parts = []
  if (queue.pending > 0) parts.push(`${queue.pending} waiting`)
  if (queue.processing > 0) parts.push(`${queue.processing} running`)
  return parts.join(', ') || 'Idle'
}

function HorizonSaveControls({
  horizon,
  draft,
  pending,
  onSave,
}: {
  horizon: MemoryHorizon
  draft?: string
  pending: boolean
  onSave: (content: string) => void
}) {
  const value = draft ?? horizon.content
  const dirty = draft !== undefined && draft !== horizon.content
  return (
    <div className="flex items-center gap-3 pb-1">
      <span className="text-[12px] text-ink-3">{value.length} chars</span>
      {dirty || pending ? (
        <Button variant="primary" size="sm" onClick={() => onSave(value)} disabled={pending}>
          {pending ? 'Saving...' : 'Save'}
        </Button>
      ) : null}
    </div>
  )
}
