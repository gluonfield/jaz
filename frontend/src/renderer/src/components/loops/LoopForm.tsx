import { useQuery } from '@tanstack/react-query'
import { type ReactNode, useEffect, useMemo, useState } from 'react'
import { MentionSuggestions, MentionTextarea, useMentionInput } from '@/components/session/MentionInput'
import { ModelSelect, RuntimeSelect } from '@/components/session/NewThreadControls'
import { boardsQuery } from '@/lib/api/boards'
import { agentSettingsQuery } from '@/lib/api/settings'
import { enabledACPAgents, runtimeModelState } from '@/lib/agentRuntimes'
import {
  acpAgentModelSuggestions,
  modelProviderModelsQuery,
  modelSuggestionsForProvider,
} from '@/lib/models'
import {
  effectiveReasoningEffort,
  modelReasoningEffortOptions,
  supportedReasoningEffort,
} from '@/lib/reasoningEfforts'
import { BoardAssignmentPicker } from './BoardAssignmentPicker'
import { LoopExamplesPicker } from './LoopExamplesPicker'
import type { LoopDraft } from './loopDraft'
import { templatePatch } from './loopTemplates'
import { SchedulePicker } from './SchedulePicker'

// A patch updater shared by every step: merges a partial draft into the whole.
type SetDraft = (patch: Partial<LoopDraft>) => void

// Borderless like the composer: the surface fill is the field. A primary ring
// appears only on focus, never as a resting outline.
const inputClass =
  'w-full rounded-control bg-surface px-3 py-2 text-[13px] text-ink outline-none transition duration-150 placeholder:text-ink-3 focus:ring-1 focus:ring-primary'

function Field({ label, children }: { label: ReactNode; children: ReactNode }) {
  return (
    <label className="block">
      <span className="mb-1.5 block text-[12px] font-medium text-ink-2">{label}</span>
      {children}
    </label>
  )
}

// The explanatory line above each step's input, with an optional right-aligned
// action (the Prompt step hangs its Examples button here).
function StepHeader({ description, action }: { description: string; action?: ReactNode }) {
  return (
    <div className="flex items-center justify-between gap-3">
      <p className="text-pretty text-[13px] text-ink-2">{description}</p>
      {action}
    </div>
  )
}

// Step 1 — the instruction: a mention-capable prompt, examples to seed it, and
// an optional name.
export function PromptStep({
  draft,
  disabled,
  autoFocus,
  description,
  set,
}: {
  draft: LoopDraft
  disabled?: boolean
  autoFocus?: boolean
  description: string
  set: SetDraft
}) {
  // The prompt seeds its text at mount, so applying an example must remount the
  // card — bumping `seed` does exactly that.
  const [seed, setSeed] = useState(0)
  const [examplesOpen, setExamplesOpen] = useState(false)

  return (
    <div className="space-y-3">
      <StepHeader
        description={description}
        action={
          <button
            type="button"
            disabled={disabled}
            onClick={() => setExamplesOpen(true)}
            className="shrink-0 text-[12px] font-medium text-primary transition-colors hover:text-primary-strong disabled:opacity-50"
          >
            Examples
          </button>
        }
      />
      <LoopPromptCard key={seed} draft={draft} disabled={disabled} autoFocus={autoFocus} set={set} />
      <LoopExamplesPicker
        open={examplesOpen}
        onClose={() => setExamplesOpen(false)}
        onPick={(template) => {
          set(templatePatch(template))
          setSeed((s) => s + 1)
          setExamplesOpen(false)
        }}
      />

      <Field label={<>Name <span className="font-normal text-ink-3">optional</span></>}>
        <input
          type="text"
          disabled={disabled}
          value={draft.name}
          onChange={(e) => set({ name: e.target.value })}
          placeholder="daily-code-review"
          className={inputClass}
        />
      </Field>
    </div>
  )
}

// Step 2 — when it runs.
export function ScheduleStep({
  draft,
  disabled,
  description,
  set,
}: {
  draft: LoopDraft
  disabled?: boolean
  description: string
  set: SetDraft
}) {
  return (
    <div className="space-y-4">
      <StepHeader description={description} />
      <SchedulePicker
        value={draft.schedule}
        disabled={disabled}
        onChange={(schedule) => set({ schedule })}
      />
    </div>
  )
}

// Step 3 — which boards carry the loop's live widget.
export function BoardsStep({
  draft,
  disabled,
  description,
  set,
}: {
  draft: LoopDraft
  disabled?: boolean
  description: string
  set: SetDraft
}) {
  const boards = useQuery(boardsQuery)

  const body =
    boards.isPending ? (
      <span className="text-[12px] text-ink-3">Loading boards…</span>
    ) : boards.isError || (boards.data ?? []).length === 0 ? (
      <p className="text-[12px] text-ink-3">
        No boards yet — create one with the + next to Boards in the sidebar to give this loop a
        live widget.
      </p>
    ) : (
      <BoardAssignmentPicker
        boards={boards.data}
        selected={draft.boardIds}
        disabled={disabled}
        onChange={(boardIds) => set({ boardIds })}
      />
    )

  return (
    <div className="space-y-4">
      <StepHeader description={description} />
      {body}
    </div>
  )
}

// The composer-style prompt card: a mention-capable textarea ($skill / @file)
// with the run setup (agent, model) as its toolbar. The draft keeps `directory`
// — it scopes @-file mentions — even though there's no project picker; keep it.
function LoopPromptCard({
  draft,
  disabled,
  autoFocus,
  set,
}: {
  draft: LoopDraft
  disabled?: boolean
  autoFocus?: boolean
  set: SetDraft
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
  const runtimeReady = settingsQuery.isSuccess
  const runtimeAvailable = runtimeReady && agents.length > 0

  useEffect(() => {
    if (!runtimeReady) return
    if (agents.includes(draft.runtime)) return
    const runtime = agents[0] ?? ''
    if (runtime === draft.runtime) return
    set({ runtime, provider: '', model: '', reasoningEffort: '' })
  }, [agents, draft.runtime, runtimeReady, set])

  const runtimeModel = runtimeModelState(agentSettings, draft.runtime, draft.provider)
  const { usesProvider, providers: runtimeProviders, provider, selectedProvider } = runtimeModel
  const defaultModel = runtimeModel.defaultModel
  const model = draft.model || defaultModel

  const providerModels = useQuery({
    ...modelProviderModelsQuery(provider),
    enabled: usesProvider && Boolean(provider),
  })
  const modelSuggestions = usesProvider
    ? modelSuggestionsForProvider(selectedProvider, providerModels.data ?? [])
    : acpAgentModelSuggestions(agentSettings, draft.runtime)
  const effortOptions = modelReasoningEffortOptions(agentSettings, draft.runtime, model, modelSuggestions)
  const reasoningEffort = effectiveReasoningEffort(
    draft.reasoningEffort || runtimeModel.defaultEffort,
    effortOptions,
  )

  useEffect(() => {
    if (draft.reasoningEffort && !supportedReasoningEffort(draft.reasoningEffort, effortOptions)) {
      set({ reasoningEffort: '' })
      return
    }
    if (
      draft.reasoningEffort === '' &&
      runtimeModel.defaultEffort &&
      !supportedReasoningEffort(runtimeModel.defaultEffort, effortOptions) &&
      effortOptions.some((option) => option.value === 'none')
    ) {
      set({ reasoningEffort: 'none' })
    }
  }, [draft.reasoningEffort, effortOptions, runtimeModel.defaultEffort, set])

  return (
    <div>
      <div className="relative">
        <MentionSuggestions mention={mention} placement="above" />
        <div
          className="flex cursor-text flex-col gap-2 rounded-card bg-surface p-2.5"
          onClick={(e) => {
            if ((e.target as HTMLElement).closest('button, textarea, input')) return
            mention.textareaRef.current?.focus()
          }}
        >
          <MentionTextarea
            mention={mention}
            placeholder="Review yesterday's commits and flag anything concerning…"
            disabled={disabled}
            autoFocus={autoFocus}
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
                  disabled={disabled}
                  placement="below"
                  onChange={(runtime) => set({ runtime, provider: '', model: '', reasoningEffort: '' })}
                />
                <ModelSelect
                  value={model}
                  suggestions={modelSuggestions}
                  loading={providerModels.isLoading}
                  disabled={disabled}
                  placement="below"
                  onChange={(next) => set({ model: next, reasoningEffort: '' })}
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
                  effortOptions={effortOptions}
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
        Type $ to tag a skill, @ to tag a file or thread.
      </span>
    </div>
  )
}
