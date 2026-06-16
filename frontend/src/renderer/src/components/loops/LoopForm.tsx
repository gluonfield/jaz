import { useQuery } from '@tanstack/react-query'
import { Check, LayoutGrid } from 'lucide-react'
import { type ReactNode, useEffect, useMemo } from 'react'
import { MentionSuggestions, MentionTextarea, useMentionInput } from '@/components/session/MentionInput'
import { ModelSelect, RuntimeSelect } from '@/components/session/NewThreadControls'
import { boardsQuery } from '@/lib/api/boards'
import type { LoopInput } from '@/lib/api/loops'
import { agentSettingsQuery } from '@/lib/api/settings'
import type { AgentSettings, Loop } from '@/lib/api/types'
import {
  acpUsesModelProvider,
  acpUsesNativeProvider,
  enabledACPAgents,
  configuredNativeProviders,
  runtimeModelState,
} from '@/lib/agentRuntimes'
import {
  acpAgentModelSuggestions,
  modelSuggestionsForProvider,
  OPENAI_MODELS,
  openRouterModelsQuery,
} from '@/lib/models'
import { acpReasoningEffortOptions, REASONING_EFFORT_OPTIONS } from '@/lib/reasoningEfforts'
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
    runtime: 'jaz',
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
    runtime: loop.runtime === 'acp' ? (loop.acp_agent || 'jaz') : 'native',
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
  if (draft.runtime.trim() === '') return false
  if (draft.schedule.preset === 'custom' && draft.schedule.expr.trim() === '') return false
  return true
}

export function loopDraftToInput(draft: LoopDraft, settings?: AgentSettings): LoopInput {
  const native = draft.runtime === 'native'
  const usesNativeProvider = native || acpUsesNativeProvider(settings, draft.runtime)
  const usesModelProvider = !native && acpUsesModelProvider(settings, draft.runtime)
  return {
    prompt: draft.prompt.trim(),
    name: draft.name.trim() || undefined,
    schedule: { kind: 'cron', expr: cronFromDraft(draft.schedule), timezone: localTimezone() },
    status: draft.schedule.preset === 'manual' ? 'paused' : 'active',
    runtime: native ? 'native' : 'acp',
    acp_agent: native ? undefined : draft.runtime,
    // Overrides are always sent: '' clears one back to following settings.
    model_provider: usesNativeProvider || usesModelProvider ? draft.provider : '',
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

// Like Field but a plain block — used for groups of controls (the schedule
// picker, the board pills) where a <label> would forward hover/click to one child.
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
  disabled,
  onChange,
}: {
  draft: LoopDraft
  disabled?: boolean
  onChange: (next: LoopDraft) => void
}) {
  const set = (patch: Partial<LoopDraft>) => onChange({ ...draft, ...patch })

  return (
    <div className="space-y-5">
      <LoopPromptCard
        draft={draft}
        disabled={disabled}
        set={set}
      />

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

// The composer-style prompt card: a mention-capable textarea ($skill / @file)
// with the loop's run setup — runtime and model — as its toolbar. The UI no
// longer offers a project picker (new loops default to the workspace), but a
// loop's `directory` is still honored end-to-end — it can be set via the
// API/MCP and is round-tripped on edit — so the draft deliberately keeps
// reading and sending it. Don't drop that plumbing.
function LoopPromptCard({
  draft,
  disabled,
  set,
}: {
  draft: LoopDraft
  disabled?: boolean
  set: (patch: Partial<LoopDraft>) => void
}) {
  const mention = useMentionInput({
    fileRoot: draft.directory,
    disabled,
    maxHeight: 240,
    initialValue: draft.prompt,
    onValueChange: (prompt) => set({ prompt }),
  })

  // Resolve the Settings > Agents defaults so the picker always shows the
  // model and effort a run will actually use — never an opaque "Default".
  const settingsQuery = useQuery(agentSettingsQuery)
  const agentSettings = settingsQuery.data
  const agents = useMemo(() => enabledACPAgents(agentSettings), [agentSettings])
  const nativeProviders = useMemo(() => configuredNativeProviders(agentSettings), [agentSettings])
  const nativeAvailable = nativeProviders.length > 0
  const runtimeReady = settingsQuery.isSuccess
  const runtimeAvailable = runtimeReady && (nativeAvailable || agents.length > 0)

  useEffect(() => {
    if (!runtimeReady) return
    const valid = draft.runtime === 'native' ? nativeAvailable : agents.includes(draft.runtime)
    if (valid) return
    const runtime = agents.includes('jaz') ? 'jaz' : nativeAvailable ? 'native' : (agents[0] ?? '')
    if (runtime === draft.runtime) return
    set({ runtime, provider: '', model: '', reasoningEffort: '' })
  }, [agents, draft.runtime, nativeAvailable, runtimeReady, set])

  const runtimeModel = runtimeModelState(agentSettings, draft.runtime, draft.provider)
  const { usesNativeProvider, usesProvider, providers: runtimeProviders, provider, selectedProvider } = runtimeModel
  const defaultModel = runtimeModel.defaultModel
  const model = draft.model || defaultModel
  const reasoningEffort = draft.reasoningEffort || runtimeModel.defaultEffort

  const openRouterModels = useQuery({
    ...openRouterModelsQuery,
    enabled: usesProvider && provider === 'openrouter',
  })
  const modelSuggestions = usesProvider
    ? usesNativeProvider && provider === 'openai'
      ? OPENAI_MODELS
      : modelSuggestionsForProvider(selectedProvider, openRouterModels.data ?? [])
    : acpAgentModelSuggestions(draft.runtime)

  return (
    <div>
      <div className="relative">
        <MentionSuggestions mention={mention} placement="below" />
        <div
          className="flex cursor-text flex-col gap-2 rounded-card bg-surface p-2.5 ring-1 ring-border transition duration-150 focus-within:ring-primary"
          onClick={(e) => {
            if ((e.target as HTMLElement).closest('button, textarea, input')) return
            mention.textareaRef.current?.focus()
          }}
        >
          <MentionTextarea
            mention={mention}
            placeholder="Review yesterday's commits and flag anything concerning…"
            disabled={disabled}
            minHeightClass="min-h-[54px]"
          />
          <div className="flex flex-wrap items-center gap-1.5">
            {runtimeReady && !runtimeAvailable ? (
              <span className="px-1.5 text-[13px] text-ink-3">Connect an agent in Settings</span>
            ) : null}
            {runtimeAvailable ? (
              <>
                <RuntimeSelect
                  value={draft.runtime}
                  agents={agents}
                  nativeAvailable={nativeAvailable}
                  disabled={disabled}
                  placement="below"
                  onChange={(runtime) => set({ runtime, provider: '', model: '', reasoningEffort: '' })}
                />
                <ModelSelect
                  value={model}
                  suggestions={modelSuggestions}
                  loading={openRouterModels.isLoading}
                  disabled={disabled}
                  placement="below"
                  onChange={(next) => set({ model: next })}
                  providers={
                    usesProvider
                      ? runtimeProviders.map((p) => ({ value: p.id, label: p.label }))
                      : undefined
                  }
                  provider={usesProvider ? provider : undefined}
                  onProviderChange={
                    usesProvider
                      ? (next) => set({ provider: next, model: '', reasoningEffort: '' })
                      : undefined
                  }
                  effort={reasoningEffort}
                  effortOptions={
                    usesNativeProvider
                      ? REASONING_EFFORT_OPTIONS
                      : acpReasoningEffortOptions(agentSettings, draft.runtime)
                  }
                  // 'Default' clears the override; the selection snaps back to the
                  // resolved settings effort.
                  onEffortChange={(next) => set({ reasoningEffort: next })}
                />
              </>
            ) : null}
          </div>
        </div>
      </div>
      <span className="mt-1.5 block text-[12px] text-ink-3">
        Type $ to tag a skill, @ to tag a file.
      </span>
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
              className={`flex h-7 items-center gap-1.5 rounded-full px-3 text-[12px] font-medium ring-1 transition duration-150 active:scale-[0.97] disabled:opacity-50 ${
                active
                  ? 'bg-surface-2 text-ink ring-border/60 shadow-sm'
                  : 'text-ink-2 ring-border hover:bg-surface hover:text-ink'
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
