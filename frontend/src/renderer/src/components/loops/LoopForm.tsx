import { useQuery } from '@tanstack/react-query'
import { ChevronDown } from 'lucide-react'
import { type ReactNode, useEffect, useMemo, useState } from 'react'
import { MentionSuggestions, MentionTextarea, useMentionInput } from '@/components/session/MentionInput'
import { ModelSelect, RuntimeSelect } from '@/components/session/NewThreadControls'
import { boardsQuery } from '@/lib/api/boards'
import { agentSettingsQuery } from '@/lib/api/settings'
import { enabledACPAgents, runtimeModelState } from '@/lib/agentRuntimes'
import {
  acpAgentModelSuggestions,
  modelSuggestionsForProvider,
  openRouterModelsQuery,
} from '@/lib/models'
import { acpReasoningEffortOptions } from '@/lib/reasoningEfforts'
import { BoardAssignmentPicker } from './BoardAssignmentPicker'
import { LoopExamples } from './LoopExamples'
import type { LoopDraft } from './loopDraft'
import { templatePatch } from './loopTemplates'
import { SchedulePicker } from './SchedulePicker'

// A patch updater shared by every step: merges a partial draft into the whole.
type SetDraft = (patch: Partial<LoopDraft>) => void

// Borderless like the composer: the surface fill is the field. A primary ring
// appears only on focus, never as a resting outline.
const inputClass =
  'w-full rounded-control bg-surface px-3 py-2 text-[13px] text-ink outline-none transition duration-150 placeholder:text-ink-3 focus:ring-1 focus:ring-primary'

function Field({ label, hint, children }: { label: string; hint?: string; children: ReactNode }) {
  return (
    <label className="block">
      <span className="mb-1.5 block text-[12px] font-medium text-ink-2">{label}</span>
      {children}
      {hint ? <span className="mt-1 block text-[12px] text-ink-3">{hint}</span> : null}
    </label>
  )
}

// Step 1 — the instruction: a mention-capable prompt, inline examples to seed
// it, and an optional name.
export function PromptStep({
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
  // The prompt seeds its text at mount, so applying an example must remount the
  // card — bumping `seed` does exactly that.
  const [seed, setSeed] = useState(0)
  const [showExamples, setShowExamples] = useState(false)

  return (
    <div className="space-y-4">
      <LoopPromptCard key={seed} draft={draft} disabled={disabled} autoFocus={autoFocus} set={set} />

      <div>
        <button
          type="button"
          disabled={disabled}
          onClick={() => setShowExamples((v) => !v)}
          className="flex items-center gap-1 text-[12px] font-medium text-ink-2 transition-colors hover:text-ink disabled:opacity-50"
        >
          <ChevronDown
            size={13}
            className={`transition-transform duration-150 ${showExamples ? 'rotate-180' : ''}`}
          />
          Examples
        </button>
        {showExamples ? (
          <div className="mt-2">
            <LoopExamples
              onPick={(template) => {
                set(templatePatch(template))
                setSeed((s) => s + 1)
                setShowExamples(false)
              }}
            />
          </div>
        ) : null}
      </div>

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
    </div>
  )
}

// Step 2 — when it runs.
export function ScheduleStep({
  draft,
  disabled,
  set,
}: {
  draft: LoopDraft
  disabled?: boolean
  set: SetDraft
}) {
  return (
    <SchedulePicker
      value={draft.schedule}
      disabled={disabled}
      onChange={(schedule) => set({ schedule })}
    />
  )
}

// Step 3 — which boards carry the loop's live widget.
export function BoardsStep({
  draft,
  disabled,
  set,
}: {
  draft: LoopDraft
  disabled?: boolean
  set: SetDraft
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

  return (
    <BoardAssignmentPicker
      boards={boards.data}
      selected={draft.boardIds}
      disabled={disabled}
      onChange={(boardIds) => set({ boardIds })}
    />
  )
}

// The composer-style prompt card: a mention-capable textarea ($skill / @file)
// with the loop's run setup - agent and model - as its toolbar. The UI no
// longer offers a project picker (new loops default to the workspace), but a
// loop's `directory` is still honored end-to-end — it can be set via the
// API/MCP and is round-tripped on edit — so the draft deliberately keeps
// reading and sending it. Don't drop that plumbing.
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
    const runtime = agents.includes('jaz') ? 'jaz' : (agents[0] ?? '')
    if (runtime === draft.runtime) return
    set({ runtime, provider: '', model: '', reasoningEffort: '' })
  }, [agents, draft.runtime, runtimeReady, set])

  const runtimeModel = runtimeModelState(agentSettings, draft.runtime, draft.provider)
  const { usesProvider, providers: runtimeProviders, provider, selectedProvider } = runtimeModel
  const defaultModel = runtimeModel.defaultModel
  const model = draft.model || defaultModel
  const reasoningEffort = draft.reasoningEffort || runtimeModel.defaultEffort

  const openRouterModels = useQuery({
    ...openRouterModelsQuery,
    enabled: usesProvider && provider === 'openrouter',
  })
  const modelSuggestions = usesProvider
    ? modelSuggestionsForProvider(selectedProvider, openRouterModels.data ?? [])
    : acpAgentModelSuggestions(draft.runtime)

  return (
    <div>
      <div className="relative">
        <MentionSuggestions mention={mention} placement="below" />
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
                  effortOptions={acpReasoningEffortOptions(agentSettings, draft.runtime)}
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
