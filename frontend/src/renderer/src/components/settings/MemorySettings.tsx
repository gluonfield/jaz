import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { MarkdownEditor } from '@/components/agent/MarkdownEditor'
import { FileTabs } from './FileTabs'
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
import type { MemoryHorizon, MemoryStatus, MemoryTask } from '@/lib/api/types'
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
  const status = useQuery(memoryQuery)
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

  const setDreamAgent = useMutation({
    mutationFn: (dream_agent: string) => updateMemorySettings({ dream_agent }),
    onSuccess: setStatus,
    onError: (error: Error) => toast(`Couldn't update dream agent: ${error.message}`, 'danger'),
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
  const dreamAgents = enabledACPAgents(agentSettings.data)
  const selectedDreamAgent = memory.dream_agent ?? ''
  const staleDreamAgent = selectedDreamAgent && !dreamAgents.includes(selectedDreamAgent)
  const dreamAgentOptions = [
    { value: '', label: 'Not selected', description: 'Use provider-backed dream when available' },
    ...(staleDreamAgent
      ? [
          {
            value: selectedDreamAgent,
            label: `${agentLabel(selectedDreamAgent)} (disabled)`,
            description: 'Enable this ACP agent or choose another one',
          },
        ]
      : []),
    ...dreamAgents.map((agent) => ({
      value: agent,
      label: agentLabel(agent),
      description: 'Run dream through this ACP agent',
    })),
  ]
  const dreamAgentValid =
    !selectedDreamAgent || dreamAgents.includes(selectedDreamAgent) || agentSettings.isPending

  return (
    <div className="mx-auto flex max-w-[860px] flex-col gap-5 px-10 pb-8 pt-2">
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
        <section className="rounded-card border border-border bg-surface">
          <div className="flex items-center justify-between border-b border-border px-4 py-3">
            <div className="flex items-baseline gap-3 text-[13px] text-ink">
              <span className="font-medium">{memory.doctor.page_count} pages</span>
              <span className="text-ink-2">{memory.doctor.chunk_count} chunks</span>
              <span className="text-ink-2">{memory.doctor.link_count} links</span>
              <span className="text-ink-2">{memory.doctor.typed_link_count} typed</span>
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
        </section>

        <section className="rounded-card border border-border bg-surface px-4 py-3">
          <div className="flex items-center justify-between gap-4">
            <div className="min-w-0">
              <span className="text-[13px] font-medium text-ink">Dream agent</span>
              <p className="mt-0.5 text-[12px] text-ink-2">
                Coding agent used for memory consolidation.
              </p>
              {!dreamAgentValid ? (
                <p className="mt-1 text-[12px] text-danger">
                  {agentLabel(memory.dream_agent)} is no longer enabled.
                </p>
              ) : null}
            </div>
            <Select
              value={selectedDreamAgent}
              options={dreamAgentOptions}
              disabled={setDreamAgent.isPending || agentSettings.isPending}
              onChange={(dreamAgent) => setDreamAgent.mutate(dreamAgent)}
              aria-label="Dream agent"
              className="w-full md:w-[260px]"
            />
          </div>
        </section>

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
            <div className="h-64 overflow-hidden rounded-card border border-border">
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
            </div>
          </section>
        ) : null}

        {memory.mcp_url ? (
          <section className="rounded-card border border-border bg-surface px-4 py-3">
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
          </section>
        ) : null}
      </div>
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
  const over = value.length > horizon.max_chars
  return (
    <div className="flex items-center gap-3 pb-1">
      <span className={`text-[12px] ${over ? 'font-medium text-danger' : 'text-ink-3'}`}>
        {value.length}/{horizon.max_chars}
      </span>
      {dirty || pending ? (
        <Button variant="primary" size="sm" onClick={() => onSave(value)} disabled={pending}>
          {pending ? 'Saving…' : 'Save'}
        </Button>
      ) : null}
    </div>
  )
}
