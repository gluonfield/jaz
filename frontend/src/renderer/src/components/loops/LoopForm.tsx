import { useQuery } from '@tanstack/react-query'
import { Check, LayoutGrid } from 'lucide-react'
import type { ReactNode } from 'react'
import { ModelSelect, ProjectPicker, RuntimeSelect } from '@/components/session/NewThreadControls'
import { boardsQuery } from '@/lib/api/boards'
import type { LoopInput } from '@/lib/api/loops'
import { agentSettingsQuery } from '@/lib/api/settings'
import type { Loop } from '@/lib/api/types'
import { acpAgentModelSuggestions, OPENAI_MODELS, openRouterModelsQuery } from '@/lib/models'
import { SchedulePicker } from './SchedulePicker'
import {
  type ScheduleDraft,
  cronFromDraft,
  defaultScheduleDraft,
  draftFromLoop,
  localTimezone,
} from './schedule'

// 'native' selects the native runtime; any other value is the ACP agent name —
// matching the RuntimeSelect contract used by the new-thread composer.
export interface LoopDraft {
  name: string
  prompt: string
  runtime: string
  directory: string
  // Overrides of the Settings > Agents defaults; '' follows settings at run
  // time, while the picker always displays the resolved effective value.
  provider: string
  model: string
  reasoningEffort: string
  schedule: ScheduleDraft
  // Boards the loop's widget lives on; assignment is the widget enablement.
  boardIds: string[]
}

export function emptyLoopDraft(boardIds: string[] = []): LoopDraft {
  return {
    name: '',
    prompt: '',
    runtime: 'native',
    directory: '',
    provider: '',
    model: '',
    reasoningEffort: '',
    schedule: defaultScheduleDraft(),
    boardIds,
  }
}

export function loopDraftFromLoop(loop: Loop, boardIds: string[] = []): LoopDraft {
  return {
    name: loop.name ?? '',
    prompt: loop.prompt ?? '',
    runtime: loop.runtime === 'acp' ? (loop.acp_agent || 'codex') : 'native',
    directory: loop.directory ?? '',
    provider: loop.model_provider ?? '',
    model: loop.model ?? '',
    reasoningEffort: loop.reasoning_effort ?? '',
    schedule: draftFromLoop(loop.schedule?.expr ?? '', loop.status === 'paused'),
    boardIds,
  }
}

export function canSaveLoop(draft: LoopDraft): boolean {
  if (draft.prompt.trim() === '') return false
  if (draft.schedule.preset === 'custom' && draft.schedule.expr.trim() === '') return false
  return true
}

export function loopDraftToInput(draft: LoopDraft): LoopInput {
  const native = draft.runtime === 'native'
  return {
    prompt: draft.prompt.trim(),
    name: draft.name.trim() || undefined,
    schedule: { kind: 'cron', expr: cronFromDraft(draft.schedule), timezone: localTimezone() },
    status: draft.schedule.preset === 'manual' ? 'paused' : 'active',
    runtime: native ? 'native' : 'acp',
    acp_agent: native ? undefined : draft.runtime,
    // Overrides are always sent: '' clears one back to following settings.
    model_provider: native ? draft.provider : '',
    model: draft.model,
    reasoning_effort: draft.reasoningEffort,
    directory: draft.directory || undefined,
    // Always sent: an empty list unassigns the widget from every board.
    board_ids: draft.boardIds,
  }
}

const inputClass =
  'w-full rounded-control bg-bg px-3 py-2 text-[13px] text-ink ring-1 ring-border outline-none transition duration-150 placeholder:text-ink-3 focus:ring-primary'

function Field({ label, hint, children }: { label: string; hint?: string; children: ReactNode }) {
  return (
    <label className="block">
      <span className="mb-1.5 block text-[12px] font-medium text-ink-2">{label}</span>
      {children}
      {hint ? <span className="mt-1 block text-[12px] text-ink-3">{hint}</span> : null}
    </label>
  )
}

// Like Field but a plain block — used for groups of controls (the agent pills,
// the schedule picker) where a <label> would forward hover/click to one child.
function FieldGroup({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div>
      <span className="mb-1.5 block text-[12px] font-medium text-ink-2">{label}</span>
      {children}
    </div>
  )
}

export function LoopForm({
  draft,
  agents,
  disabled,
  onChange,
}: {
  draft: LoopDraft
  agents: string[]
  disabled?: boolean
  onChange: (next: LoopDraft) => void
}) {
  const set = (patch: Partial<LoopDraft>) => onChange({ ...draft, ...patch })

  // Resolve the Settings > Agents defaults so the picker always shows the
  // model and effort a run will actually use — never an opaque "Default".
  const { data: agentSettings } = useQuery(agentSettingsQuery)
  const isNative = draft.runtime === 'native'
  const defaultProvider = agentSettings?.native.model_provider ?? ''
  const provider = draft.provider || defaultProvider
  const defaultModel = isNative
    ? provider === defaultProvider
      ? (agentSettings?.native.model ?? '')
      : (agentSettings?.providers.find((p) => p.id === provider)?.default_model ?? '')
    : (agentSettings?.acp[draft.runtime]?.model ?? '')
  const model = draft.model || defaultModel
  const defaultEffort = isNative
    ? (agentSettings?.native.reasoning_effort ?? '')
    : (agentSettings?.acp[draft.runtime]?.reasoning_effort ?? '')
  const reasoningEffort = draft.reasoningEffort || defaultEffort

  const openRouterModels = useQuery({
    ...openRouterModelsQuery,
    enabled: isNative && provider === 'openrouter',
  })
  const modelSuggestions = isNative
    ? provider === 'openrouter'
      ? (openRouterModels.data ?? [])
      : OPENAI_MODELS
    : acpAgentModelSuggestions(draft.runtime)

  return (
    <div className="space-y-5">
      <Field label="Name" hint="Optional — defaults to the start of the prompt.">
        <input
          type="text"
          disabled={disabled}
          value={draft.name}
          onChange={(e) => set({ name: e.target.value })}
          placeholder="daily-code-review"
          className={inputClass}
        />
      </Field>

      <Field label="Prompt" hint="Sent to a fresh thread on each run.">
        <textarea
          rows={4}
          disabled={disabled}
          value={draft.prompt}
          onChange={(e) => set({ prompt: e.target.value })}
          placeholder="Review yesterday's commits and flag anything concerning…"
          className={`${inputClass} resize-y`}
        />
      </Field>

      <FieldGroup label="Agent">
        <div className="flex flex-wrap items-center gap-2">
          <RuntimeSelect
            value={draft.runtime}
            agents={agents}
            disabled={disabled}
            onChange={(runtime) =>
              set({ runtime, provider: '', model: '', reasoningEffort: '' })
            }
          />
          <ModelSelect
            value={model}
            suggestions={modelSuggestions}
            loading={openRouterModels.isLoading}
            disabled={disabled}
            onChange={(next) => set({ model: next })}
            providers={
              isNative
                ? (agentSettings?.providers ?? [])
                    .filter((p) => p.implemented)
                    .map((p) => ({ value: p.id, label: p.label }))
                : undefined
            }
            provider={isNative ? provider : undefined}
            onProviderChange={
              isNative
                ? (next) => set({ provider: next, model: '', reasoningEffort: '' })
                : undefined
            }
            effort={reasoningEffort}
            // 'Default' clears the override; the selection snaps back to the
            // resolved settings effort.
            onEffortChange={(next) => set({ reasoningEffort: next })}
          />
          <ProjectPicker
            value={draft.directory}
            disabled={disabled}
            onChange={(directory) => set({ directory })}
          />
        </div>
      </FieldGroup>

      <FieldGroup label="Schedule">
        <SchedulePicker
          value={draft.schedule}
          disabled={disabled}
          onChange={(schedule) => set({ schedule })}
        />
      </FieldGroup>

      <FieldGroup label="Boards">
        <BoardPicker
          selected={draft.boardIds}
          disabled={disabled}
          onChange={(boardIds) => set({ boardIds })}
        />
      </FieldGroup>
    </div>
  )
}

// Assigning the loop to boards is what turns its widget on: each run then
// refreshes a live tile on every selected board.
function BoardPicker({
  selected,
  disabled,
  onChange,
}: {
  selected: string[]
  disabled?: boolean
  onChange: (boardIds: string[]) => void
}) {
  const boards = useQuery(boardsQuery)

  if (boards.isPending) {
    return <span className="text-[12px] text-ink-3">Loading boards…</span>
  }
  if (boards.isError || (boards.data ?? []).length === 0) {
    return (
      <p className="text-[12px] text-ink-3">
        No boards yet — create one with the + next to Boards in the sidebar to give this loop a
        live widget.
      </p>
    )
  }

  const toggle = (id: string) =>
    onChange(selected.includes(id) ? selected.filter((b) => b !== id) : [...selected, id])

  return (
    <div>
      <div className="flex flex-wrap items-center gap-1.5">
        {boards.data.map((board) => {
          const active = selected.includes(board.id)
          return (
            <button
              key={board.id}
              type="button"
              disabled={disabled}
              aria-pressed={active}
              onClick={() => toggle(board.id)}
              className={`flex items-center gap-1.5 rounded-full px-3 py-1 text-[12px] font-medium ring-1 transition-colors duration-150 disabled:opacity-50 ${
                active
                  ? 'bg-primary-soft text-primary-strong ring-primary/40'
                  : 'bg-bg text-ink-2 ring-border hover:text-ink'
              }`}
            >
              {active ? <Check size={12} /> : <LayoutGrid size={12} />}
              {board.name}
            </button>
          )
        })}
      </div>
      <span className="mt-1.5 block text-[12px] text-ink-3">
        On every run the loop refreshes a live widget on the selected boards.
      </span>
    </div>
  )
}
