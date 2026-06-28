import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
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
  updateMemoryEnabled,
  updateMemorySettings,
} from '@/lib/api/memory'
import type { MemoryHorizon, MemoryQueueStatus, MemoryStatus, MemoryTask } from '@/lib/api/types'
import { enabledACPAgents } from '@/lib/agentRuntimes'
import { keys } from '@/lib/query/keys'

const HORIZON_DESCRIPTIONS: Record<string, string> = {
  'LONG_TERM.md':
    'Who you are and where you are going: identity, goals, standing preferences. Rewritten by dream; agents receive it through memory context.',
  'SHORT_TERM.md':
    'What is true right now: current focus, active projects, open loops. Agents update it live; dream prunes stale entries.',
}

const TASK_LABELS: Record<string, string> = {
  index_changed_pages: 'Index',
  ingest_sources: 'Ingest sources',
  daily_rollup: 'Daily page',
  link_hygiene: 'Link hygiene',
  dream: 'Dream',
  optimize_index: 'Optimize index',
}

function formatTime(value?: string): string {
  if (!value) return 'never'
  const date = new Date(value)
  if (Number.isNaN(date.getTime()) || date.getTime() <= 0) return 'never'
  return date.toLocaleString(undefined, {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })
}

export function MemorySettings() {
  const status = useQuery({ ...memoryQuery, refetchInterval: 5000 })
  const agentSettings = useQuery(agentSettingsQuery)
  const queryClient = useQueryClient()
  const toast = useToast()

  const [activeHorizon, setActiveHorizon] = useState<string>()
  const [drafts, setDrafts] = useState<Record<string, string>>({})

  const setStatus = (next: MemoryStatus) => queryClient.setQueryData(keys.memory, next)

  const toggle = useMutation({
    mutationFn: updateMemoryEnabled,
    onSuccess: setStatus,
    onError: (error: Error) => toast(`Couldn't update memory: ${error.message}`, 'danger'),
  })

  const setMemoryAgent = useMutation({
    mutationFn: (agent: string) => updateMemorySettings({ agent }),
    onSuccess: setStatus,
    onError: (error: Error) => toast(`Couldn't update memory agent: ${error.message}`, 'danger'),
  })

  const reindex = useMutation({
    mutationFn: reindexMemory,
    onSuccess: (report) => {
      toast(`Indexed ${report.page_count} pages, ${report.chunk_count} chunks`)
      void queryClient.invalidateQueries({ queryKey: keys.memory })
    },
    onError: (error: Error) => toast(`Index failed: ${error.message}`, 'danger'),
  })

  const runDream = useMutation({
    mutationFn: dreamMemory,
    onSuccess: (result) => {
      toast(result.dream.run_slug ? `Dream finished: ${result.dream.run_slug}` : 'Dream finished')
      void queryClient.invalidateQueries({ queryKey: keys.memory })
    },
    onError: (error: Error) => toast(`Dream failed: ${error.message}`, 'danger'),
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

  return (
    <div className="flex flex-col gap-5 py-5">
      <header className="flex items-start justify-between gap-4">
        <div>
          <h1 className="text-lg font-semibold text-ink">Memory</h1>
          <p className="mt-0.5 max-w-[58ch] text-[13px] text-ink-2">
            Markdown-first personal memory. When enabled, agents receive long-term and
            short-term context, capture into daily pages, and dream consolidates periodically.
          </p>
        </div>
        <div className="flex h-8 shrink-0 items-center gap-2">
          <span className="text-[12px] text-ink-2">{enabled ? 'Enabled' : 'Disabled'}</span>
          <Switch
            checked={enabled}
            disabled={toggle.isPending}
            onChange={(next) => toggle.mutate(next)}
            aria-label="Enable memory"
          />
        </div>
      </header>

      <div className={enabled ? 'flex flex-col gap-5' : 'pointer-events-none flex flex-col gap-5 opacity-50'}>
        <SettingsCard>
          <div className="flex items-center justify-between border-b border-border px-4 py-3">
            <div className="flex items-baseline gap-3 text-[13px] text-ink">
              <span className="font-medium tabular-nums">{memory.doctor.page_count} pages</span>
              <span className="tabular-nums text-ink-2">{memory.doctor.chunk_count} chunks</span>
              <span className="tabular-nums text-ink-2">{memory.doctor.link_count} links</span>
              <span className="tabular-nums text-ink-2">{memory.doctor.typed_link_count} typed</span>
            </div>
            <div className="flex items-center gap-2">
              <Button
                variant="secondary"
                size="sm"
                onClick={() => reindex.mutate()}
                disabled={reindex.isPending || runDream.isPending}
              >
                {reindex.isPending ? 'Indexing…' : 'Index now'}
              </Button>
              <Button
                variant="secondary"
                size="sm"
                onClick={() => runDream.mutate()}
                disabled={reindex.isPending || runDream.isPending}
              >
                {runDream.isPending ? 'Dreaming…' : 'Run dream'}
              </Button>
            </div>
          </div>
          {memory.source_queues ? (
            <div className="grid grid-cols-1 divide-y divide-border border-b border-border md:grid-cols-2 md:divide-x md:divide-y-0">
              <SourceQueueStatus
                label="Source projection"
                detail="Raw provider data to materialized source files."
                queue={memory.source_queues.projection}
              />
              <SourceQueueStatus
                label="Memory capture"
                detail="Materialized source files reserved for jazmem."
                queue={memory.source_queues.memory}
              />
            </div>
          ) : null}
          <ul className="divide-y divide-border">
            {memory.tasks.map((task: MemoryTask) => (
              <li key={task.name} className="flex items-center justify-between gap-3 px-4 py-2">
                <div className="min-w-0">
                  <span className="text-[13px] text-ink">{TASK_LABELS[task.name] ?? task.name}</span>
                  {task.error ? (
                    <p className="truncate text-[12px] text-danger" title={task.error}>
                      {task.error}
                    </p>
                  ) : null}
                </div>
                <div className="flex shrink-0 items-center gap-3 text-[12px] text-ink-2">
                  <span
                    className={
                      task.status === 'error' ? 'font-medium text-danger' : task.status ? '' : 'text-ink-3'
                    }
                  >
                    {task.status ? `${task.status} ${formatTime(task.last_run_at)}` : 'never ran'}
                  </span>
                  <span className="text-ink-3">next {formatTime(task.next_due)}</span>
                </div>
              </li>
            ))}
          </ul>
        </SettingsCard>

        <SettingsCard className="px-4 py-3">
          <div className="grid grid-cols-1 gap-3 md:grid-cols-[minmax(0,1fr)_260px] md:items-center">
            <div className="min-w-0">
              <span className="text-[13px] font-medium text-ink">Memory agent</span>
              <p className="mt-0.5 text-[12px] text-ink-2">
                Coding agent used by memory_search and dream.
              </p>
              {!memoryAgentValid ? (
                <p className="mt-1 text-[12px] text-danger">
                  {agentLabel(selectedMemoryAgent)} is no longer enabled.
                </p>
              ) : null}
            </div>
            <Select
              value={selectedMemoryAgent}
              options={memoryAgentOptions}
              disabled={setMemoryAgent.isPending || agentSettings.isPending}
              onChange={(agent) => setMemoryAgent.mutate(agent)}
              aria-label="Memory agent"
            />
          </div>
        </SettingsCard>

        {active ? (
          <section className="flex flex-col">
            <div className="flex items-end justify-between gap-4 border-b border-border">
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
            <p className="pb-2 text-[12px] text-ink-2">{HORIZON_DESCRIPTIONS[active.name]}</p>
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

        {memory.mcp_url ? (
          <SettingsCard className="px-4 py-3">
            <div className="flex items-center justify-between gap-3">
              <div>
                <span className="text-[13px] font-medium text-ink">MCP endpoint</span>
                <p className="text-[12px] text-ink-2">
                  Served to ACP agents (codex, claude) automatically while memory is enabled.
                </p>
              </div>
              <code className="shrink-0 rounded bg-surface-2 px-2 py-1 font-mono text-[12px] text-ink-2">
                {memory.mcp_url}
              </code>
            </div>
          </SettingsCard>
        ) : null}
      </div>
    </div>
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
  const active = queue.dirty > 0 || queue.processing > 0
  return (
    <div className="min-w-0 px-4 py-3">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <span className="block text-[12px] font-medium text-ink">{label}</span>
          <span className="mt-0.5 block truncate text-[11px] text-ink-3">{detail}</span>
        </div>
        <span
          className={`mt-0.5 shrink-0 rounded-full px-2 py-0.5 text-[10px] font-medium ${
            queue.error
              ? 'bg-danger-soft text-danger'
              : active
                ? 'bg-primary-soft text-primary-strong'
                : 'bg-surface-2 text-ink-3'
          }`}
        >
          {queue.error ? 'Error' : active ? 'Active' : 'Idle'}
        </span>
      </div>
      <div className="mt-2 flex flex-wrap items-center gap-x-4 gap-y-1 text-[12px] text-ink-2">
        <span>
          <span className="font-mono tabular-nums text-ink">{queue.dirty}</span> dirty
        </span>
        <span>
          <span className="font-mono tabular-nums text-ink">{queue.processing}</span> processing
        </span>
      </div>
      {queue.error ? (
        <p className="mt-1 truncate text-[11px] text-danger" title={queue.error}>
          {queue.error}
        </p>
      ) : null}
    </div>
  )
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
          {pending ? 'Saving…' : 'Save'}
        </Button>
      ) : null}
    </div>
  )
}
